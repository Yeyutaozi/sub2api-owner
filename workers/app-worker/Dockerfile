FROM python:3.12-slim

ARG PIP_INDEX_URL=https://mirrors.aliyun.com/pypi/simple
ARG PIP_EXTRA_INDEX_URL=
ARG PIP_DEFAULT_TIMEOUT=120
ARG PIP_RETRIES=5

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/app/src
ENV PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /app

COPY requirements.txt .
RUN set -eux; \
    if [ -n "${PIP_EXTRA_INDEX_URL}" ]; then \
        pip install --no-cache-dir \
            --index-url "${PIP_INDEX_URL}" \
            --extra-index-url "${PIP_EXTRA_INDEX_URL}" \
            --default-timeout "${PIP_DEFAULT_TIMEOUT}" \
            --retries "${PIP_RETRIES}" \
            -r requirements.txt; \
    else \
        pip install --no-cache-dir \
            --index-url "${PIP_INDEX_URL}" \
            --default-timeout "${PIP_DEFAULT_TIMEOUT}" \
            --retries "${PIP_RETRIES}" \
            -r requirements.txt; \
    fi

COPY src ./src
COPY worker.yaml .

EXPOSE 8091

CMD ["uvicorn", "sub2api_worker.main:app", "--host", "0.0.0.0", "--port", "8091"]
