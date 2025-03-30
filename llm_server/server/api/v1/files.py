# server/api/v1/files.py

import os
import hashlib
import uuid
import time
import shutil
import base64
from typing import List, Dict, Any, Optional, Tuple
from fastapi import APIRouter, UploadFile, File, HTTPException, Form, BackgroundTasks, Request
from fastapi.responses import JSONResponse

from tools.database.file import create_file, find_existing_file_by_hash, update_file_by_content_hash, find_files_by_content_hash
from tools.parse.parser import Parse
from tools.logger import logger

router = APIRouter()

# Maximum file size (256MB)
MAX_FILE_SIZE = 256 * 1024 * 1024  

# Get supported file extensions from the parser
parser = Parse()
SUPPORTED_EXTENSIONS = list(parser.file_type_parsers.keys())

# Upload directory
UPLOAD_DIR = "/app/uploads"
os.makedirs(UPLOAD_DIR, exist_ok=True)
logger.info(f"Using uploads directory: {UPLOAD_DIR}")

async def save_file_to_disk(file_content: bytes, filename: str) -> Tuple[str, str]:
    """
    Save a file to disk and return the file path using file-uuid.ext format.
    
    Args:
        file_content: The content of the file
        filename: The name of the file
        
    Returns:
        Tuple[str, str]: (full_path, filename_only) 
    """
    # Generate a unique uuid
    file_uuid = str(uuid.uuid4())
    
    # Get file extension
    _, ext = os.path.splitext(filename)
    
    # Create filename in format file-uuid.ext
    safe_filename = f"file-{file_uuid}{ext}"
    
    # Create the full path
    file_path = os.path.join(UPLOAD_DIR, safe_filename)
    
    # Ensure uploads directory exists
    os.makedirs(UPLOAD_DIR, exist_ok=True)
    
    # Write the file to disk
    with open(file_path, "wb") as f:
        f.write(file_content)
    
    logger.info(f"File saved to disk: {file_path}")
    
    return file_path, safe_filename

def is_text_file(file_ext: str) -> bool:
    """
    Determine if a file extension typically represents a text file.
    
    Args:
        file_ext: File extension including the dot (e.g., '.txt', '.py')
        
    Returns:
        bool: True if it's a text file, False otherwise
    """
    text_extensions = [
        '.txt', '.py', '.java', '.js', '.html', '.css', '.c', '.cpp', 
        '.h', '.hpp', '.cs', '.php', '.rb', '.go', '.rs', '.sql', 
        '.ts', '.swift', '.kt', '.csv', '.tsv', '.json', '.xml', 
        '.yaml', '.yml', '.md', '.rst'
    ]
    return file_ext.lower() in text_extensions

@router.post("", response_class=JSONResponse)
async def upload_files(
    request: Request,
    background_tasks: BackgroundTasks = None,
    files: List[UploadFile] = File(...),
    author_id: Optional[str] = Form(None),
    author_type: Optional[str] = Form(None)
):
    """
    Upload one or more files and store them in the database.
    
    Args:
        request: The FastAPI request object
        files: List of files to upload
        background_tasks: FastAPI BackgroundTasks for async processing
        author_id: Optional ID of the author
        author_type: Optional type of the author (user, bot, etc.)
        
    Returns:
        JSONResponse: Response with file IDs or error message
    """
    logger.info(f"Received file upload request: {request.url}")
    
    if not files:
        return JSONResponse(
            status_code=400,
            content={
                "status": 400,
                "message": "No files provided",
                "result": []
            }
        )
    
    # Set up author if provided
    author = None
    if author_id and author_type:
        author = {"id": author_id, "type": author_type}
    
    result = []
    all_successful = True
    error_message = ""
    
    for file in files:
        try:
            # Check file size
            file_content = await file.read()
            size = len(file_content)
            
            if size > MAX_FILE_SIZE:
                error_message = f"File {file.filename} is too large, max file size 256MB"
                all_successful = False
                continue
            
            # Check file extension
            _, file_ext = os.path.splitext(file.filename.lower())
            if file_ext not in SUPPORTED_EXTENSIONS:
                error_message = f"Unsupported file type: {file_ext}"
                all_successful = False
                continue
            
            # Calculate content hash (SHA-256) from the binary content
            binary_content_hash = hashlib.sha256(file_content).hexdigest()
            logger.debug(f"Calculated binary content hash for {file.filename}: {binary_content_hash[:8]}...")
            
            # Check for duplicates
            logger.info(f"Checking if file with hash {binary_content_hash[:8]}... already exists")
            existing_file = find_existing_file_by_hash(binary_content_hash)
            
            file_id = None
            file_path = None
            stored_filename = None
            
            if existing_file:
                logger.info(f"Found existing file with matching hash: {existing_file.get('file_id')}")
                
                if existing_file.get("address") != "deleted":
                    # File exists and is not deleted, update timestamp
                    logger.info(f"Updating existing file (not deleted): {existing_file.get('file_id')}")
                    updated_file = update_file_by_content_hash(binary_content_hash, existing_file.get("address"))
                    file_id = updated_file.get("file_id")
                    stored_filename = existing_file.get("address")
                else:
                    # File exists but was deleted, restore it
                    logger.info(f"Restoring previously deleted file: {existing_file.get('file_id')}")
                    # Save file to disk
                    file_path, stored_filename = await save_file_to_disk(file_content, file.filename)
                    logger.info(f"Saved restored file to disk: {stored_filename}")
                    updated_file = update_file_by_content_hash(binary_content_hash, stored_filename)
                    file_id = updated_file.get("file_id")
            
            # If no existing file found, create a new one
            if not file_id:
                logger.info(f"No existing file found with matching hash, creating new file record")
                
                # Save file to disk
                file_path, stored_filename = await save_file_to_disk(file_content, file.filename)
                logger.info(f"Saved new file to disk: {stored_filename}")
                
                # Determine MIME type
                mime_type = file.content_type or "application/octet-stream"
                logger.debug(f"File MIME type: {mime_type}")
                
                # For non-text files, store empty string as content
                file_content_text = ""
                is_text = is_text_file(file_ext)
                
                if is_text:
                    # Try to decode as text if it's a text file
                    try:
                        logger.debug(f"Detected text file, attempting to decode content")
                        file_content_text = file_content.decode('utf-8', errors='ignore')
                        # Calculate text content hash for verification
                        text_content_hash = hashlib.sha256(file_content_text.encode()).hexdigest()
                        logger.debug(f"Text content hash: {text_content_hash[:8]}...")
                        
                        # Compare with binary hash
                        if text_content_hash != binary_content_hash:
                            logger.warning(f"Text content hash differs from binary content hash:")
                            logger.warning(f"  - Binary hash: {binary_content_hash[:16]}...")
                            logger.warning(f"  - Text hash:   {text_content_hash[:16]}...")
                            logger.warning(f"This may cause hash mismatch errors when retrieving the file")
                    except Exception as decode_err:
                        logger.error(f"Error decoding text file: {str(decode_err)}")
                        file_content_text = ""
                else:
                    logger.debug(f"Binary file detected, storing empty content string")
                
                # Create file record in database
                new_file = create_file(
                    filename=file.filename,
                    file_type=mime_type,
                    size=size,
                    content=file_content_text,  # Only text content or empty string
                    author=author,
                    address=stored_filename,  # Just the filename, not the full path
                    content_hash=binary_content_hash  # Pass the pre-computed hash
                )
                
                file_id = new_file.get("file_id")
                logger.info(f"Created new file record with ID: {file_id}")
            
            # Add to result list
            result.append({
                "file-id": file_id,
                "size": size,
                "filename": file.filename,
                "stored_filename": stored_filename
            })
            
            logger.info(f"Successfully processed file: {file.filename}, file_id: {file_id}")
            
        except Exception as e:
            logger.error(f"Error processing file {file.filename}: {str(e)}")
            error_message = f"Error processing file {file.filename}: {str(e)}"
            all_successful = False
            
            # Reset file position for other potential uses
            await file.seek(0)
    
    # Prepare response
    if all_successful:
        return JSONResponse(
            content={
                "status": 200,
                "message": "success",
                "result": result
            }
        )
    else:
        # If some files failed but others succeeded
        if result:
            return JSONResponse(
                status_code=207,  # Multi-Status
                content={
                    "status": 207,
                    "message": error_message,
                    "result": result
                }
            )
        else:
            # If all files failed
            return JSONResponse(
                status_code=400,
                content={
                    "status": 400,
                    "message": error_message,
                    "result": []
                }
            )

@router.get("/debug/file/{file_id}", response_class=JSONResponse)
async def debug_file_path(file_id: str):
    """
    Debug endpoint to check file paths and access.
    
    Args:
        file_id: The ID of the file to check
        
    Returns:
        JSONResponse: File path information and access status
    """
    try:
        logger.info(f"Debug request for file ID: {file_id}")
        
        # Get file details from database
        file_details = get_file(file_id)
        
        if not file_details:
            return JSONResponse(
                status_code=404,
                content={
                    "status": 404,
                    "message": f"File not found: {file_id}",
                }
            )
        
        # Extract file details
        filename = file_details.get("filename", "unknown")
        file_type = file_details.get("type", "unknown")
        address = file_details.get("address", "unknown")
        size = file_details.get("size", 0)
        content_hash = file_details.get("content_hash", "unknown")
        has_content = bool(file_details.get("content", ""))
        content_length = len(file_details.get("content", "")) if has_content else 0
        
        # Check standard path
        standard_path = os.path.join(UPLOAD_DIR, address)
        standard_path_exists = os.path.exists(standard_path)
        
        # Check alternate paths
        alt_paths = [
            os.path.join("/tmp/uploads", address),
            os.path.join("/var/tmp/uploads", address),
            address  # Try using address directly if it's an absolute path
        ]
        
        alt_paths_status = []
        for path in alt_paths:
            exists = os.path.exists(path)
            alt_paths_status.append({
                "path": path,
                "exists": exists,
                "size": os.path.getsize(path) if exists else None
            })
        
        # If standard path exists, check if we can read it
        file_readable = False
        file_content_start = ""
        
        if standard_path_exists:
            try:
                with open(standard_path, 'rb') as f:
                    content_bytes = f.read(100)  # Read first 100 bytes
                    file_readable = True
                    try:
                        file_content_start = content_bytes.decode('utf-8', errors='replace')
                    except:
                        file_content_start = str(content_bytes)
            except Exception as e:
                file_readable = False
                file_content_start = f"Error reading file: {str(e)}"
        
        # Return detailed information
        return JSONResponse(
            content={
                "status": 200,
                "message": "File details retrieved",
                "file": {
                    "file_id": file_id,
                    "filename": filename,
                    "file_type": file_type,
                    "size": size,
                    "content_hash": content_hash,
                    "address": address,
                    "database_content": {
                        "has_content": has_content,
                        "content_length": content_length
                    },
                    "standard_path": {
                        "path": standard_path,
                        "exists": standard_path_exists,
                        "readable": file_readable,
                        "content_start": file_content_start if standard_path_exists and file_readable else None
                    },
                    "alternate_paths": alt_paths_status
                }
            }
        )
    except Exception as e:
        logger.error(f"Error in debug_file_path: {str(e)}")
        return JSONResponse(
            status_code=500,
            content={
                "status": 500,
                "message": f"Error checking file paths: {str(e)}",
            }
        ) 