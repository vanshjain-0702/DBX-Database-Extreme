# syntax=docker/dockerfile:1
FROM node:22-alpine AS frontend
WORKDIR /build/jurix/frontend
COPY jurix/frontend/package.json jurix/frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY jurix/frontend/ ./
RUN npm run build

FROM python:3.13-slim-bookworm AS dependencies
WORKDIR /build
COPY jurix/requirements.txt ./requirements.txt
# Wheels avoid adding compilers, Rust, or development headers to the runtime.
RUN python -m pip install --no-cache-dir --only-binary=:all: --prefix=/install -r requirements.txt

FROM python:3.13-slim-bookworm AS runtime
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/jurix:/app/legal-auditor:/app/sdk/python \
    JURIX_ENV=production JURIX_DATABASE_PATH=/data/jurix.sqlite3
RUN groupadd --gid 10001 jurix \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin jurix \
    && mkdir -p /data /app/jurix /app/legal-auditor /app/sdk/python /app/website/assets \
    && chown 10001:10001 /data \
    && chmod 0700 /data
WORKDIR /app
COPY --from=dependencies /install /usr/local
# Explicit copies keep local registries, uploads, and credentials out of the image.
COPY jurix/*.py /app/jurix/
COPY jurix/deploy/*.py /app/jurix/deploy/
COPY legal-auditor/*.py /app/legal-auditor/
COPY sdk/python/dbx.py /app/sdk/python/dbx.py
COPY website/assets/ /app/website/assets/
COPY --from=frontend /build/jurix/static/ /app/jurix/static/
USER 10001:10001
EXPOSE 8091
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD ["python", "jurix/deploy/healthcheck.py", "app"]
ENTRYPOINT ["python", "jurix/deploy/entrypoint.py"]
CMD ["python", "-m", "uvicorn", "app:app", "--app-dir", "jurix", "--host", "0.0.0.0", "--port", "8091", "--workers", "1", "--limit-concurrency", "64", "--timeout-keep-alive", "5", "--timeout-graceful-shutdown", "30", "--no-server-header", "--no-access-log", "--forwarded-allow-ips", "172.30.70.2"]
