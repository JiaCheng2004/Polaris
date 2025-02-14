# docker/Dockerfile
FROM ubuntu:24.04

# Install dependencies
RUN apt-get update && apt-get install -y \
    cmake \
    g++ \
    make \
    git \
    libssl-dev \
    build-essential \
    libboost-all-dev \
    nlohmann-json3-dev \
    libcurl4-openssl-dev \
 && rm -rf /var/lib/apt/lists/*

# Create a working directory
WORKDIR /app

# Copy everything into the container (adjust as needed)
COPY . /app

# Build the project
RUN mkdir -p build && cd build && cmake .. && make

# Expose the server port (adjust if your app listens on a different port)
EXPOSE 8080

# By default, run the server when starting the container
CMD ["/app/build/llm_server"]
