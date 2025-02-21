FROM ubuntu:24.04

# Disable interactive prompts (e.g. tzdata)
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=America/Los_Angeles
ENV APP_NAME="llm_server"

# [1] Install system dependencies
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
    libjsoncpp-dev \
    sqlite3 \
    libsqlite3-dev \
    uuid-dev \
    zlib1g-dev \
  && ln -snf /usr/share/zoneinfo/$TZ /etc/localtime \
  && echo $TZ > /etc/timezone \
  && rm -rf /var/lib/apt/lists/*

# [2] Build & install Drogon from source (with submodules)
RUN git clone --recursive --branch v1.9.9 \
      https://github.com/drogonframework/drogon.git /tmp/drogon \
 && cd /tmp/drogon \
 && mkdir build && cd build \
 && cmake -DCMAKE_INSTALL_PREFIX=/usr .. \
 && make -j$(nproc) \
 && make install \
 && rm -rf /tmp/drogon

# [3] Create folder + log file
RUN mkdir -p /var/log/${APP_NAME} \
  && touch /var/log/${APP_NAME}/server.log \
  && chmod 666 /var/log/${APP_NAME}/server.log

# [4] Workdir & copy sources
WORKDIR /app
COPY . /app

# [5] Build your project
RUN mkdir -p build && cd build \
 && cmake .. \
 && make -j$(nproc)

# [6] Expose & run
EXPOSE 8080
CMD ["/app/build/llm_server"]
