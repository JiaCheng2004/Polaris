import os
import json
import base64
import requests
import mimetypes

class GeminiVideoParser():
    def __init__(self, api_key=None):
        """Initialize the Gemini Video parser.
        
        Args:
            api_key (str, optional): Gemini API key. If not provided, will try to get from environment.
        """
        self.api_key = api_key or os.environ.get("GEMINI_API_KEY")
        if not self.api_key:
            raise ValueError("Gemini API key is required. Set GEMINI_API_KEY environment variable or pass to constructor.")
        
        # Define supported video formats and their MIME types
        self.supported_formats = {
            '.flv': 'video/x-flv',
            '.mov': 'video/quicktime',
            '.mpeg': 'video/mpeg',
            '.mpegps': 'video/mpeg',
            '.mpg': 'video/mpeg',
            '.mp4': 'video/mp4',
            '.webm': 'video/webm',
            '.wmv': 'video/x-ms-wmv',
            '.3gpp': 'video/3gpp'
        }
        
    def parse(self, file_path: str) -> str:
        """
        Parse video files using Google's Gemini API.
        Supports multiple video formats.
        
        Args:
            file_path (str): Path to the video file
            
        Returns:
            str: Description and analysis of the video content
        """
        
        try:
            # Get file extension and check if it's supported
            _, file_ext = os.path.splitext(file_path.lower())
            if file_ext not in self.supported_formats:
                return f"Unsupported video format: {file_ext}"
            
            mime_type = self.supported_formats[file_ext]
            
            # Read the video file as binary
            with open(file_path, "rb") as file:
                file_content = file.read()
            
            # Encode the file content as base64
            file_base64 = base64.b64encode(file_content).decode("utf-8")
            
            # Prepare the API request
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
                            "text": "Analyze this video content and provide a detailed description of what is happening, including any text, speech, and significant visual elements."
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
            
            # Make the API request
            response = requests.post(url, headers=headers, json=payload)
            response.raise_for_status()
            
            # Parse the response
            result = response.json()
            
            # Extract the content from the response
            if "candidates" in result and result["candidates"]:
                candidate = result["candidates"][0]
                if "content" in candidate and "parts" in candidate["content"]:
                    for part in candidate["content"]["parts"]:
                        if "text" in part:
                            return part["text"]
                        elif "functionResponse" in part and "response" in part["functionResponse"]:
                            data = json.loads(part["functionResponse"]["response"])
                            return data.get("file_content", "")
            
            return "Failed to extract content from the video."
            
        except Exception as e:
            return f"Error processing video: {str(e)}" 