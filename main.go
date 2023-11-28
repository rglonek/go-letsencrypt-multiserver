package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"golang.org/x/crypto/acme/autocert"
)

type stateCache struct {
	realCache autocert.Cache
	db        *sql.DB
}

func (s *stateCache) Put(ctx context.Context, name string, data []byte) error {
	if strings.HasSuffix(name, "+http-01") {
		s.db.Exec("DELETE FROM CACHE WHERE name=?", name)
		time.Sleep(time.Second)
		_, err := s.db.Exec("INSERT INTO CACHE (timestamp, name, value) VALUES (NOW(), ?, ?)", name, data)
		if err != nil {
			log.Printf("Failed DB insert: %s", err)
		}
	}
	return s.realCache.Put(ctx, name, data)
}
func (s *stateCache) Get(ctx context.Context, name string) ([]byte, error) {
	if strings.HasSuffix(name, "+http-01") {
		r := s.db.QueryRow("SELECT value FROM CACHE WHERE name=?", name)
		err := r.Err()
		if err != nil {
			return s.realCache.Get(ctx, name)
		}
		var ret []byte
		err = r.Scan(&ret)
		if err != nil {
			return s.realCache.Get(ctx, name)
		}
		return ret, nil
	}
	return s.realCache.Get(ctx, name)
}
func (s *stateCache) Delete(ctx context.Context, name string) error {
	_, err := s.db.Exec("DELETE FROM CACHE WHERE name=?", name)
	if err != nil {
		log.Printf("Failed DB delete: %s", err)
	}
	return s.realCache.Delete(ctx, name)
}

func connect(user string, pass string, dbName string, diskCache string) (autocert.Cache, error) {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@/%s", user, pass, dbName))
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(0)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	return &stateCache{
		realCache: autocert.DirCache(diskCache),
		db:        db,
	}, nil
}

func main() {
	user := flag.String("user", "autocert", "mariadb user")
	pass := flag.String("pass", "", "mariadb pass")
	dbName := flag.String("db", "autocert", "mariadb db name")
	certDir := flag.String("dir", "/opt/certs", "cert directory")
	email := flag.String("email", "", "email to use for cert")
	interval := flag.Duration("interval", time.Hour*24, "cert check interval")
	sleepStart := flag.Duration("start-sleep", 5*time.Second, "time to sleep before initially requesting certificates")
	script := flag.String("script", "", "script to run after certificate renewal")
	flag.Parse()
	domains := flag.Args()
	certs := make([][]byte, len(domains))
	if len(domains) == 0 {
		log.Fatal("Usage: autocert -user x -name x -db x -dir x -email x -script /path/to/x -- domain1.com domain2.com domain3.com ...")
	}
	cache, err := connect(*user, *pass, *dbName, *certDir)
	if err != nil {
		log.Fatal(err)
	}
	m := &autocert.Manager{
		Cache:      cache,
		Prompt:     autocert.AcceptTOS,
		Email:      *email,
		HostPolicy: autocert.HostWhitelist(domains...),
	}
	go func() {
		err := http.ListenAndServe("0.0.0.0:80", m.HTTPHandler(nil))
		if err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(*sleepStart)
	for {
		changed := false
		log.Println("Starting refresh")
		for i, domain := range domains {
			log.Printf("Getting %s", domain)
			cert, err := m.GetCertificate(&tls.ClientHelloInfo{
				ServerName: domain,
			})
			if err != nil {
				log.Printf("failed to get cert for %s: %s", domain, err)
				continue
			}
			sum := sha256.New()
			for _, b := range cert.Certificate {
				sum.Write(b)
			}
			sumx := sum.Sum(nil)
			if !bytes.Equal(certs[i], sumx) {
				certs[i] = sumx
				changed = true
			}
		}
		if changed {
			if *script != "" {
				log.Println("Certs changed, running script")
				out, err := exec.Command("/bin/bash", "-c", *script).CombinedOutput()
				if err != nil {
					log.Printf("SCRIPT FAILED: %s: %s", err, string(out))
				}
			} else {
				log.Println("Certs changed, no script")
			}
		}
		log.Println("Refresh complete")
		time.Sleep(*interval)
	}
}
