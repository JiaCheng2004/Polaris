import os
import json
import base64
import requests
import mimetypes

class GeminiImageParser():
    def __init__(self, api_key=None):
        """Initialize the Gemini Image parser.

        Args:
            api_key (str, optional): Gemini API key. If not provided, will try to get from environment.
        """
        self.api_key = api_key or os.environ.get("GEMINI_API_KEY")
        if not self.api_key:
            raise ValueError("Gemini API key is required. Set GEMINI_API_KEY environment variable or pass to constructor.")

        self.supported_formats = {
            '.png': 'image/png',
            '.jpg': 'image/jpeg',
            '.jpeg': 'image/jpeg',
            '.webp': 'image/webp'
        }

    def parse(self, file_path: str) -> str:
        """
        Parse images using Google's Gemini API.
        Supports PNG, JPEG, and WebP formats.

        Args:
            file_path (str): Path to the image file

        Returns:
            str: Extracted text/content from the image
        """

        try:

            _, file_ext = os.path.splitext(file_path.lower())
            if file_ext not in self.supported_formats:
                return f"Unsupported image format: {file_ext}"

            mime_type = self.supported_formats[file_ext]

            with open(file_path, "rb") as file:
                file_content = file.read()

            file_base64 = base64.b64encode(file_content).decode("utf-8")

            url = f"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key={self.api_key}"
            headers = {
                "Content-Type": "application/json"
            }

            payload = {
                "contents": [
                    {
                        "role": "user",
                        "parts": [
                            {
                                "inlineData": {
                                    "mimeType": mime_type,
                                    "data": file_base64
                                }
                            }
                        ]
                    }
                ],
                "systemInstruction": {
                    "role": "user",
                    "parts": [
                        {
                            "text": "Extract all text and describe the content visible in this image in detail."
                        }
                    ]
                },
                "generationConfig": {
                    "temperature": 0.2,
                    "topK": 40,
                    "topP": 0.95,
                    "maxOutputTokens": 8192,
                    "responseMimeType": "application/json",
                    "responseSchema": {
                        "type": "object",
                        "properties": {
                            "file_content": {
                                "type": "string"
                            }
                        }
                    }
                }
            }

            response = requests.post(url, headers=headers, json=payload)
            response.raise_for_status()

            result = response.json()

            if "candidates" in result and result["candidates"]:
                candidate = result["candidates"][0]
                if "content" in candidate and "parts" in candidate["content"]:
                    for part in candidate["content"]["parts"]:
                        if "text" in part:
                            return part["text"]
                        elif "functionResponse" in part and "response" in part["functionResponse"]:
                            data = json.loads(part["functionResponse"]["response"])
                            return data.get("file_content", "")

            return "Failed to extract content from the image."

        except Exception as e:
            return f"Error processing image: {str(e)}"