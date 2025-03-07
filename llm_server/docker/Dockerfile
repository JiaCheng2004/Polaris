# docker/Dockerfile

# Use Ubuntu 24.04 as the base image
FROM ubuntu:24.04

# Set environment variables
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=America/Los_Angeles

# Update and install system dependencies
RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y --no-install-recommends \
        python3 \
        python3-venv \
        python3-pip \
        curl \
        build-essential \
        ca-certificates \
        tzdata \
    && rm -rf /var/lib/apt/lists/*

# Set the container timezone properly
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && \
    echo $TZ > /etc/timezone

# Create a working directory
WORKDIR /app

# Copy the entire project into the container
COPY . /app

# Create a Python virtual environment
RUN python3 -m venv /app/venv

# Install Python dependencies in the virtual environment
RUN /app/venv/bin/pip install --upgrade pip && \
    /app/venv/bin/pip install -r /app/requirements.txt

# Expose port 8080 for the server
EXPOSE 8080

# Set the default command to run your FastAPI server
# using Uvicorn on port 8080
CMD ["/app/venv/bin/uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8080"]
