FROM caddy:2-alpine
RUN addgroup -g 10003 jurixproxy \
    && adduser -D -H -u 10003 -G jurixproxy jurixproxy \
    && mkdir -p /data/caddy /config/caddy \
    && chown -R 10003:10003 /data /config
USER 10003:10003
EXPOSE 8443
