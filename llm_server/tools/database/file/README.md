# File Database Operations

This module provides functions to manage files in the database. Files are standalone entities that are not directly linked to messages or threads.

## Key Features

- Files are independent entities - they don't belong to any message or thread
- Files track their physical storage location with the `address` field
- When a file is physically deleted, its record remains with address="deleted"
- Content hashing allows identifying duplicate files
- When a file with the same content hash is uploaded again, its address can be updated

## Database Schema

Files are stored in the `files` table with the following structure:

```sql
CREATE TABLE files (
    file_id      TEXT PRIMARY KEY DEFAULT generate_prefixed_uuid('file'),
    author       JSONB NOT NULL,                -- JSON describing who uploaded or generated it
    filename     TEXT NOT NULL,                 -- file name
    type         TEXT NOT NULL,                 -- MIME type (e.g. application/pdf, image/png)
    size         INT  NOT NULL,                 -- file size in bytes
    token_count  INT  NOT NULL DEFAULT 0,       -- tokens extracted (if applicable)
    content      TEXT,                          -- potentially long string
    metadata     JSONB NOT NULL DEFAULT '{}',   -- extra metadata
    content_hash TEXT NOT NULL,                 -- hash for integrity checks
    address      TEXT NOT NULL,                 -- path to physical file or "deleted" if removed
    parse_tool   JSONB NOT NULL DEFAULT '{}',   -- e.g. {"type": "api", "name": "gemini"}
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

## Usage Examples

### Creating a New File

```python
from llm_server.tools.database.file import create_file

file_data = create_file(
    filename="example.pdf",
    file_type="application/pdf",
    size=1024,
    content="...",  # File content as text
    author={"id": "user-123", "name": "John Doe"},
    address="/path/to/physical/file.pdf"
)
print(f"Created file with ID: {file_data['file_id']}")
```

### Retrieving a File

```python
from llm_server.tools.database.file import get_file

file_data = get_file("file-123abc", include_content=False)
print(f"File name: {file_data['filename']}")
print(f"Storage location: {file_data['address']}")
```

### Listing Files

```python
from llm_server.tools.database.file import list_files

# List all PDF files
pdf_files = list_files(file_type="application/pdf")

# List by content hash (finding duplicates)
duplicate_files = list_files(content_hash="ab123...")
```

### Updating Files

```python
from llm_server.tools.database.file import update_file, mark_file_as_deleted

# Update metadata
updated_file = update_file(
    file_id="file-123abc",
    metadata={"processed": True, "page_count": 10}
)

# Mark a file as deleted
deleted_file = mark_file_as_deleted("file-123abc")
print(f"File marked as deleted: {deleted_file['address'] == 'deleted'}")
```

### Handling Re-uploads of Deleted Files

```python
from llm_server.tools.database.file import find_existing_file_by_hash, update_file_address

# Calculate the content hash of the uploaded file
import hashlib
content_hash = hashlib.sha256(content.encode()).hexdigest()

# Check if a file with this hash already exists
existing_file = find_existing_file_by_hash(content_hash)

if existing_file:
    if existing_file['address'] == 'deleted':
        # File was previously deleted, update its address to the new location
        updated_file = update_file_address(
            file_id=existing_file['file_id'],
            address="/path/to/new/file.pdf"
        )
        print("Re-uploaded file with new location")
    else:
        print("File already exists at:", existing_file['address'])
else:
    # Create a new file record
    new_file = create_file(...)
``` 