import os
import json
import base64
import requests
import mimetypes

class GeminiRTFDocParser():
    def __init__(self, api_key=None):
        """Initialize the Gemini Rich Text document parser.
        
        Args:
            api_key (str, optional): Gemini API key. If not provided, will try to get from environment.
        """
        self.api_key = api_key or os.environ.get("GEMINI_API_KEY")
        if not self.api_key:
            raise ValueError("Gemini API key is required. Set GEMINI_API_KEY environment variable or pass to constructor.")
        
        # Define supported rich text document formats and their MIME types
        self.supported_formats = {
            '.doc': 'application/msword',
            '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
            '.rtf': 'application/rtf',
            '.dot': 'application/msword',
            '.dotx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.template',
            '.hwp': 'application/x-hwp',
            '.hwpx': 'application/x-hwpx',
        }
        
    def parse(self, file_path: str) -> str:
        """
        Parse rich text documents using Google's Gemini API.
        Supports DOC, DOCX, RTF, DOT, DOTX, HWP, HWPX formats.
        
        Args:
            file_path (str): Path to the document file
            
        Returns:
            str: Extracted text from the document
        """
        
        try:
            # Get file extension and check if it's supported
            _, file_ext = os.path.splitext(file_path.lower())
            if file_ext not in self.supported_formats:
                return f"Unsupported document format: {file_ext}"
            
            mime_type = self.supported_formats[file_ext]
            
            # Read the document file as binary
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
                            "text": "Extract all original content from this document in plain text format, preserving structure and formatting where possible."
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
            
            return "Failed to extract content from the document."
            
        except Exception as e:
            return f"Error processing document: {str(e)}" 