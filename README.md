```
go mod init shin
go mod tidy

docker build -t shin .

docker run \
  -e TZ=Asia/Shanghai \
  -e POLL_INTERVAL=300 \
  -e FRESHRSS_AUTH_URL="" \
  -e FRESHRSS_LIST_SUBSCRIPTION_URL="" \
  -e FRESHRSS_CONTENT_URL_PREFIX="" \
  -e FRESHRSS_FILTERED_LABEL="E" \
  -e SENDER_EMAIL="" \
  -e SENDER_AUTH_TOKEN="" \
  -e SMTP_SERVER="" \
  -e SMTP_PORT=25 \
  -e RECEIVER_EMAIL="" \
  -e DEFAULT_OT="1728017013" \
  -e OT_MAP_JSON="{"feed/79": "1728017013"}" \
  shin
```