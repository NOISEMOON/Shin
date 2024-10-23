go mod init shin
go mod tidy

docker build -t shin:v4 .
docker buildx build --platform linux/arm64 -t shin:v3 --load .
docker save -o shin_v4.tar shin:v4

