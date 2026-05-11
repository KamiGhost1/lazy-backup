FROM golang:1.25.3

# Устанавливаем Node.js напрямую (без nvm для контейнеров)
# RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - \
#     && apt-get install -y nodejs


# Проверяем установку
# RUN node --version && npm --version

RUN apt-get update && apt-get install -y \
    docker.io \
    && rm -rf /var/lib/apt/lists/*


# ENV GOOS=windows
# ENV GOOS=linux
# ENV GOOS=darwin

# ENV GOARCH=amd64
# ENV GOARCH=arm64


WORKDIR /app

# CMD [ "go", "build" ]