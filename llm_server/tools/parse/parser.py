import os
import mimetypes
import json
from typing import Dict, List, Any, Optional

from .pdf import GeminiPDFParser
from .rtfdoc import GeminiRTFDocParser
from .image import GeminiImageParser
from .audio import GeminiAudioParser
from .video import GeminiVideoParser
from .textdoc import TextDocParser

from ..config.load import GOOGLE_API_KEY
from ..logger import logger

class Parse:
    """
    Universal parsing interface that selects the appropriate parser based on file type.
    Supports multiple parsing tools with fallback mechanisms.
    """

    def __init__(self, api_key: Optional[str] = None):
        """
        Initialize the parser with the API key for all tools.

        Args:
            api_key (str, optional): API key for parsing tools. If not provided, will try environment variables.
        """
        logger.info("Initializing Parse with parsers")
        self.api_key = api_key or GOOGLE_API_KEY

        self.gemini_pdf_parser = GeminiPDFParser(api_key=self.api_key)
        self.gemini_rtfdoc_parser = GeminiRTFDocParser(api_key=self.api_key)
        self.gemini_image_parser = GeminiImageParser(api_key=self.api_key)
        self.gemini_audio_parser = GeminiAudioParser(api_key=self.api_key)
        self.gemini_video_parser = GeminiVideoParser(api_key=self.api_key)
        self.text_parser = TextDocParser()
        logger.debug("All parsers initialized successfully")

        self.file_type_parsers = {

            ".pdf": [self.gemini_pdf_parser],

            ".doc": [self.gemini_rtfdoc_parser],
            ".docx": [self.gemini_rtfdoc_parser],
            ".rtf": [self.gemini_rtfdoc_parser],
            ".dot": [self.gemini_rtfdoc_parser],
            ".dotx": [self.gemini_rtfdoc_parser],
            ".hwp": [self.gemini_rtfdoc_parser],
            ".hwpx": [self.gemini_rtfdoc_parser],

            ".png": [self.gemini_image_parser],
            ".jpg": [self.gemini_image_parser],
            ".jpeg": [self.gemini_image_parser],
            ".webp": [self.gemini_image_parser],

            ".aac": [self.gemini_audio_parser],
            ".flac": [self.gemini_audio_parser],
            ".mp3": [self.gemini_audio_parser],
            ".m4a": [self.gemini_audio_parser],
            ".mpeg": [self.gemini_audio_parser, self.gemini_video_parser],  
            ".mpga": [self.gemini_audio_parser],
            ".opus": [self.gemini_audio_parser],
            ".pcm": [self.gemini_audio_parser],
            ".wav": [self.gemini_audio_parser],

            ".flv": [self.gemini_video_parser],
            ".mov": [self.gemini_video_parser],
            ".mpg": [self.gemini_video_parser],
            ".mpegps": [self.gemini_video_parser],
            ".mp4": [self.gemini_video_parser, self.gemini_audio_parser],  
            ".webm": [self.gemini_video_parser, self.gemini_audio_parser],  
            ".wmv": [self.gemini_video_parser],
            ".3gpp": [self.gemini_video_parser],

            ".txt": [self.text_parser],

            ".py": [self.text_parser],
            ".java": [self.text_parser],
            ".js": [self.text_parser],
            ".html": [self.text_parser],
            ".css": [self.text_parser],
            ".c": [self.text_parser],
            ".cpp": [self.text_parser],
            ".h": [self.text_parser],
            ".hpp": [self.text_parser],
            ".cs": [self.text_parser],
            ".php": [self.text_parser],
            ".rb": [self.text_parser],
            ".go": [self.text_parser],
            ".rs": [self.text_parser],
            ".sql": [self.text_parser],
            ".ts": [self.text_parser],
            ".swift": [self.text_parser],
            ".kt": [self.text_parser],

            ".csv": [self.text_parser],
            ".tsv": [self.text_parser],
            ".json": [self.text_parser],
            ".xml": [self.text_parser],
            ".yaml": [self.text_parser],
            ".yml": [self.text_parser],
        }
        logger.debug(f"Configured {len(self.file_type_parsers)} file type parsers")

    def __call__(self, file: str) -> Dict[str, Any]:
        """
        Parse a file and return its content.

        Args:
            file (str): Path to the file to parse

        Returns:
            Dict: Result dictionary with status, content and tools used
        """
        logger.info(f"Parse called for file: {file}")
        return self.parse(file)

    def parse(self, file_path: str) -> Dict[str, Any]:
        """
        Parse a file using the appropriate parser based on file extension.
        Will try multiple parsers in order if the first one fails.

        Args:
            file_path (str): Path to the file to parse

        Returns:
            Dict: Result dictionary containing:
                - status (int): HTTP-like status code (200 for success, 400 for error)
                - content (str): Parsed content or error message
                - tools_used (List[str]): List of tools that were tried
        """
        logger.info(f"Parsing file: {file_path}")

        if not os.path.exists(file_path):
            logger.error(f"File not found: {file_path}")
            return {
                "status": 400,
                "content": f"File not found: {file_path}",
                "tools_used": []
            }

        _, file_ext = os.path.splitext(file_path.lower())
        logger.debug(f"File extension detected: {file_ext}")

        if file_ext not in self.file_type_parsers:
            logger.error(f"Unsupported file type: {file_ext}")
            return {
                "status": 400,
                "content": f"Unsupported file type: {file_ext}",
                "tools_used": []
            }

        parsers = self.file_type_parsers[file_ext]
        tools_used = []
        logger.info(f"Found {len(parsers)} compatible parsers for {file_ext}")

        for parser in parsers:
            parser_name = parser.__class__.__name__
            tools_used.append(parser_name)
            logger.info(f"Attempting to parse with {parser_name}")

            try:
                result = parser.parse(file_path)
                logger.debug(f"Parser {parser_name} returned result type: {type(result)}")

                try:
                    if isinstance(result, str):
                        try:
                            json_result = json.loads(result)

                            if "file_content" in json_result:
                                logger.info(f"Successfully parsed file with {parser_name}")
                                return {
                                    "status": 200,
                                    "content": json_result["file_content"],
                                    "tools_used": tools_used
                                }
                            else:
                                logger.warning(f"Parser {parser_name} returned JSON without file_content")
                                continue
                        except json.JSONDecodeError:
                            logger.error(f"JSON decode error with parser {parser_name}")
                            return {
                                "status": 400,
                                "content": result,
                                "tools_used": []
                            }

                    elif isinstance(result, dict):
                        if "file_content" in result:
                            logger.info(f"Successfully parsed file with {parser_name}")
                            return {
                                "status": 200,
                                "content": result["file_content"],
                                "tools_used": tools_used
                            }
                        else:
                            logger.warning(f"Parser {parser_name} returned dict without file_content")
                            continue
                    else:
                        logger.warning(f"Parser {parser_name} returned unexpected result type")
                        continue
                except Exception as e:
                    logger.error(f"Error processing result from {parser_name}: {str(e)}")
                    return {
                        "status": 400,
                        "content": str(e),
                        "tools_used": []
                    }

            except Exception as e:
                logger.error(f"Parser {parser_name} failed with error: {str(e)}")
                continue

        logger.error(f"All parsers failed for file: {file_path}")
        return {
            "status": 400,
            "content": "All parsers failed or file type not supported correctly",
            "tools_used": tools_used
        }
