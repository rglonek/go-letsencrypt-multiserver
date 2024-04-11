GOOS_LIST=(linux darwin windows)
GOARCH_LIST=(amd64 arm64)
rm -f autocert-*.tgz
rm -f autocert-*.zip
set -e
for GOOS in ${GOOS_LIST[@]}
do
    for GOARCH in ${GOARCH_LIST[@]}
    do
        echo "Compiling and packaging for ${GOOS}:${GOARCH}"
        out=autocert
        [ "${GOOS}" = "windows" ] && out=autocert.exe
        CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build -o ${out} .
        [ "${GOOS}" != "windows" ] && tar -zcvf autocert-${GOOS}-${GOARCH}.tgz ${out}
        [ "${GOOS}" = "windows" ] && zip autocert-${GOOS}-${GOARCH}.zip ${out}
        rm -f ${out}
    done
done
echo "Done"
