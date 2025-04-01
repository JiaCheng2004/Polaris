FROM python:3.11-slim

WORKDIR /app

# Copy requirements first for better layer caching
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the bot code
COPY . .

# Create the downloads directory if it doesn't exist
RUN mkdir -p downloaded_files

# Run the bot
CMD ["python", "main.py"] 