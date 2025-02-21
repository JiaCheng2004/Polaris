FROM ubuntu:24.04

# Disable interactive prompts (e.g. tzdata)
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=America/Los_Angeles
ENV APP_NAME="bot"

# Install base dependencies (+ zlib1g-dev, libopus-dev for voice & compression)
RUN apt-get update && apt-get install -y \
    git \
    cmake \
    build-essential \
    python3 \
    python3-venv \
    curl \
    jq \
    default-jdk \
    libssl-dev \
    libcurl4-openssl-dev \
    libnacl-dev \
    libsodium-dev \
    pkg-config \
    ca-certificates \
    zlib1g-dev \
    libopus-dev \
 && rm -rf /var/lib/apt/lists/*

# ------------------------------------------------------------------------------
# 1) Clone yt-dlp (if desired) and install it in a virtual environment
# ------------------------------------------------------------------------------
# Example: from your previous attempt or the method you prefer
RUN git clone https://github.com/yt-dlp/yt-dlp.git /tmp/yt-dlp
RUN python3 -m venv /opt/venv \
 && /opt/venv/bin/pip install --upgrade pip \
 && /opt/venv/bin/pip install /tmp/yt-dlp
ENV PATH="/opt/venv/bin:$PATH"

# ------------------------------------------------------------------------------
# 2) Clone & build DPP (with voice support)
# ------------------------------------------------------------------------------
WORKDIR /tmp

RUN LATEST_RELEASE_TAG="$( \
        curl -s https://api.github.com/repos/brainboxdotcc/DPP/releases/latest \
        | jq -r .tag_name \
    )" \
 && git clone --recursive --branch "$LATEST_RELEASE_TAG" https://github.com/brainboxdotcc/DPP.git DPP \
 && cd DPP \
 && cmake -B build -S . \
      -DCMAKE_CXX_STANDARD=23 \
      -DDPP_CORO=ON \
      -DDPP_BUILD_EXAMPLES=OFF \
      -DDPP_BUILD_VOICE=ON \
 && cmake --build build --target install -- -j"$(nproc)" \
 && rm -rf /tmp/DPP

# ------------------------------------------------------------------------------
# 3) Copy your own source code and build your app
# ------------------------------------------------------------------------------
WORKDIR /app
COPY . /app

RUN mkdir -p build && cd build \
 && cmake .. -DCMAKE_CXX_STANDARD=23 \
 && cmake --build . -- -j"$(nproc)"

# ------------------------------------------------------------------------------
# 4) Final command to run your bot
# ------------------------------------------------------------------------------
CMD ["./build/marong"]
