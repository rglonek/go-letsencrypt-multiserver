# Autocert using multiple servers

This is example go application which runs on multiple servers. It runs autocert, listening and responding ton port 80 to `http-01` challenges. It redirects all other requests to `https:443` on same host.

This is achieved by using a standard disk cache for the certificates, while using a distributed database for challenges. In this example, we are using `mariadb` in master-master mode (using `galera`).

The listeners will request the certs in order and store them on disk. The challenges are stored in the distributed DB, so that any server that receives the challenge can respond, no metter who is asking for the certificate.

## Example mariadb setup

```
wget https://downloads.mariadb.com/MariaDB/mariadb_repo_setup
chmod 755 mariadb_repo_setup
./mariadb_repo_setup --os-type=rhel --os-version=9 --arch=x86_64 --skip-check-installed
yum install mariadb-server galera-4
wget -O - https://www.crushftp.com/crush10wiki/attach/Linux%20Install/configure.sh | bash
systemctl enable mariadb
systemctl stop mariadb
systemctl start mariadb
```

Then setup the galera for master-master replication:

```
vi /etc/my.cnf.d/server.cnf
[galera]
# Mandatory settings
wsrep_on=ON
wsrep_provider=/usr/lib64/galera-4/libgalera_smm.so
wsrep_cluster_address=gcomm://172.20.0.43,172.21.0.45 # this node and another
binlog_format=row
default_storage_engine=InnoDB
innodb_autoinc_lock_mode=2
wsrep_node_address=172.20.0.43 # this node
```

And start it all:

```
systemctl mariadb stop
galera_new_cluster # on first node only
systemctl mariadb start
journalctl -u mariadb -f
```

## Example create DB and tables

```
CREATE DATABASE autocert;

USE autocert;

CREATE TABLE `CACHE` (
  `timestamp` TIMESTAMP default NULL,
  `name` varchar(255) default NULL,
  `value` BLOB default NULL
) ENGINE=innodb DEFAULT CHARSET=latin1;

GRANT ALL PRIVILEGES on autocert.* TO 'autocert'@localhost IDENTIFIED BY 'DBPASS';

FLUSH PRIVILEGES;
```

## Example systemd file to star this program

```
[Unit]
Description=AutoCert
After=network.target auditd.service named.service crushftp.service
[Service]
Type=simple
ExecStart=/usr/local/bin/autocert -email useremail@example.com -pass 'DBPASS' -script /usr/local/bin/install-certs.sh example.com example.net
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
```

## Example post-renew script to launch another step

In this case, we take the certs, convert them to pkcs12 and then into java's jks store to be used by a java web server.

```
vi /usr/local/bin/install-certs.sh
set -e
cd /opt/certs
rm -rf /opt/certs.p12
mkdir /opt/certs.p12
rm -f /opt/certs.jks
ls *+rsa |awk -F'+' '{print $1}' |while read crt
do
	openssl pkcs12 -export -in ${crt}+rsa -out /opt/certs.p12/${crt} -name "${crt}" -passout pass:abcdef12
	/var/opt/CrushFTP10/Java/bin/keytool -v -importkeystore -srckeystore /opt/certs.p12/${crt} -srcstoretype PKCS12 -destkeystore /opt/certs.jks -deststoretype JKS -srcstorepass abcdef12 -deststorepass abcdef12
done
systemctl restart crushftp
```
