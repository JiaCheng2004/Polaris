import os
import json
import base64
import requests
import mimetypes

class GeminiAudioParser():
    def __init__(self, api_key=None):
        """Initialize the Gemini Audio parser.
        
        Args:
            api_key (str, optional): Gemini API key. If not provided, will try to get from environment.
        """
        self.api_key = api_key or os.environ.get("GEMINI_API_KEY")
        if not self.api_key:
            raise ValueError("Gemini API key is required. Set GEMINI_API_KEY environment variable or pass to constructor.")
        
        # Define supported audio formats and their MIME types
        self.supported_formats = {
            '.aac': 'audio/aac',
            '.flac': 'audio/flac',
            '.mp3': 'audio/mpeg',
            '.m4a': 'audio/mp4',
            '.mpeg': 'audio/mpeg',
            '.mpga': 'audio/mpeg',
            '.mp4': 'audio/mp4',
            '.opus': 'audio/opus',
            '.pcm': 'audio/pcm',
            '.wav': 'audio/wav',
            '.webm': 'audio/webm',
        }
        
    def parse(self, file_path: str) -> str:
        """
        Parse audio files using Google's Gemini API.
        Supports multiple audio formats.
        
        Args:
            file_path (str): Path to the audio file
            
        Returns:
            str: Transcription and analysis of the audio content
        """
        
        try:
            # Get file extension and check if it's supported
            _, file_ext = os.path.splitext(file_path.lower())
            if file_ext not in self.supported_formats:
                return f"Unsupported audio format: {file_ext}"
            
            mime_type = self.supported_formats[file_ext]
            
            # Read the audio file as binary
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
                            "text": "Transcribe this audio content and provide a detailed description of what you hear."
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
            
            return "Failed to extract content from the audio."
            
        except Exception as e:
            return f"Error processing audio: {str(e)}"