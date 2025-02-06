go mod init shin
go mod tidy

docker build -t shin:v5 .
docker buildx build --platform linux/arm64 -t shin:v5 --load .
docker save -o shin_v5.tar shin:v5

