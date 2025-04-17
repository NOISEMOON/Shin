go mod init shin
go mod tidy

docker build -t shin:v6 .
docker buildx build --platform linux/arm64 -t shin:v6 --load .
docker save -o shin_v6.tar shin:v6

