# Sandbox for site products that list acceptance scenarios: Chromium + Playwright (Python) from the official
# image. The platform's runner (internal/products/runner_site.py) serves the uploaded directory on 127.0.0.1
# and drives it; the container has no network.
FROM mcr.microsoft.com/playwright/python:v1.52.0-noble
RUN pip install --no-cache-dir --break-system-packages playwright==1.52.0
ENV PYTHONDONTWRITEBYTECODE=1
WORKDIR /work
