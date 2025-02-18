# docker/Dockerfile
FROM ubuntu:24.04

# --[1] Set environment variables related to tzdata --
# Disable interactive prompts during tzdata install
ENV DEBIAN_FRONTEND=noninteractive
# Set the timezone to Los Angeles (Pacific Time)
ENV TZ=America/Los_Angeles

# Name of the application
ENV APP_NAME="llm_server"

# --[2] Install dependencies, including tzdata for timezones --
RUN apt-get update && apt-get install -y \
    tzdata \
    cmake \
    g++ \
    make \
    git \
    libssl-dev \
    build-essential \
    libboost-all-dev \
    nlohmann-json3-dev \
    libcurl4-openssl-dev \
    && ln -snf /usr/share/zoneinfo/$TZ /etc/localtime \
    && echo $TZ > /etc/timezone \
    && rm -rf /var/lib/apt/lists/*

# --[3] Create folder + log file --
RUN mkdir -p /var/log/${APP_NAME} \
    && touch /var/log/${APP_NAME}/server.log \
    && chmod 666 /var/log/${APP_NAME}/server.log

# --[4] Working directory --
WORKDIR /app

# --[5] Copy sources --
COPY . /app

# --[6] Build --
RUN mkdir -p build && cd build && cmake .. && make

# --[7] Expose and run --
EXPOSE 8080
CMD /app/build/$APP_NAME
