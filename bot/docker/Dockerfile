FROM ubuntu:24.04

# ------------------------------------------------------------------------
# 0) Base environment setup
# ------------------------------------------------------------------------
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=America/Los_Angeles
ENV APP_NAME="bot"

# ------------------------------------------------------------------------
# Install system dependencies
# ------------------------------------------------------------------------
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
    wget \
 && rm -rf /var/lib/apt/lists/*

# ------------------------------------------------------------------------
# 1) Clone yt-dlp and install it in a Python virtual environment
# ------------------------------------------------------------------------
RUN git clone https://github.com/yt-dlp/yt-dlp.git /tmp/yt-dlp
RUN python3 -m venv /opt/venv \
 && /opt/venv/bin/pip install --upgrade pip \
 && /opt/venv/bin/pip install /tmp/yt-dlp

# Make the venv's bin directory available on PATH
ENV PATH="/opt/venv/bin:$PATH"

# ------------------------------------------------------------------------
# 2) Install DPP (D++) - either from prebuilt .deb or from source
# ------------------------------------------------------------------------
ARG INSTALL_DPP_METHOD=deb

RUN if [ "$INSTALL_DPP_METHOD" = "deb" ]; then \
    echo "Installing DPP from the prebuilt .deb package..." \
    && wget -O /tmp/dpp.deb https://dl.dpp.dev/ \
    && dpkg -i /tmp/dpp.deb \
    && rm -f /tmp/dpp.deb \
; else \
    echo "Installing DPP from source..." \
    && LATEST_RELEASE_TAG="$( \
        curl -s https://api.github.com/repos/brainboxdotcc/DPP/releases/latest \
        | jq -r .tag_name \
    )" \
    && git clone --recursive --branch "$LATEST_RELEASE_TAG" https://github.com/brainboxdotcc/DPP.git /tmp/DPP \
    && cd /tmp/DPP \
    && cmake -B build -S . \
         -DCMAKE_CXX_STANDARD=23 \
         -DDPP_CORO=ON \
         -DDPP_BUILD_EXAMPLES=OFF \
         -DDPP_BUILD_VOICE=ON \
    && cmake --build build --target install -- -j"$(nproc)" \
    && rm -rf /tmp/DPP \
; fi

# ------------------------------------------------------------------------
# 3) Copy your own source code and build your app
# ------------------------------------------------------------------------
WORKDIR /app
COPY . /app

RUN mkdir -p build \
 && cd build \
 && cmake .. -DCMAKE_CXX_STANDARD=23 \
 && cmake --build . -- -j"$(nproc)"

# ------------------------------------------------------------------------
# 4) Final command to run your bot
# ------------------------------------------------------------------------
CMD ["/app/build/marong"]
