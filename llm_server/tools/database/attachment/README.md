# Attachment Database Operations

This module provides CRUD (Create, Read, Update, Delete) operations for managing file attachments in the database.

## Overview

Attachments represent files associated with messages in the system. Each attachment contains:
- File metadata (name, type, size)
- The file content as text
- A content hash for integrity verification
- Token count for LLM processing metrics
- Purpose and author information

## Usage

### Import

```python
# Import specific functions
from tools.database.attachment import create_attachment, get_attachment, list_attachments, update_attachment, delete_attachment

# Or import the entire module
import tools.database.attachment as attachment_db
```

### Creating an Attachment

```python
# Example: Creating a new file attachment
author_data = {
    "user_id": "user123",
    "username": "johndoe"
}

# Create the attachment
new_attachment = create_attachment(
    message_id="abcd1234-e89b-12d3-a456-426614174000",
    filename="example.txt",
    file_type="text/plain",
    size=len("This is a sample file content"),
    content="This is a sample file content",
    author=author_data,
    purpose="reference",
    token_count=7,  # Approximate token count
    metadata={"source": "user_upload"}
)

# Access the attachment ID
file_id = new_attachment["file_id"]
print(f"Created attachment with ID: {file_id}")
```

### Retrieving Attachments

```python
# Get a specific attachment by ID (with content)
attachment = get_attachment(
    file_id="567e4567-e89b-12d3-a456-426614174000",
    include_content=True
)

# Get an attachment without its content (for large files)
attachment_metadata = get_attachment(
    file_id="567e4567-e89b-12d3-a456-426614174000",
    include_content=False
)

# List attachments for a specific message
message_attachments = list_attachments(
    message_id="abcd1234-e89b-12d3-a456-426614174000",
    include_content=False
)

# Alternatively, use the convenience function
message_attachments = get_message_attachments(
    message_id="abcd1234-e89b-12d3-a456-426614174000"
)

# Iterate through attachments
for attachment in message_attachments:
    print(f"File: {attachment['filename']} ({attachment['size']} bytes)")
```

### Updating Attachments

```python
# Update attachment metadata
updated_attachment = update_attachment(
    file_id="567e4567-e89b-12d3-a456-426614174000",
    update_data={
        "filename": "renamed_file.txt",
        "purpose": "updated-purpose",
        "metadata": {"source": "user_upload", "category": "documentation"}
    }
)

# Update file content (special function that handles hash recalculation)
updated_attachment = update_attachment_content(
    file_id="567e4567-e89b-12d3-a456-426614174000",
    content="This is the updated file content",
    update_size=True  # Automatically updates the size field based on content
)
```

### Deleting Attachments

```python
# Delete a specific attachment
success = delete_attachment(file_id="567e4567-e89b-12d3-a456-426614174000")
if success:
    print("Attachment deleted successfully")

# Delete all attachments for a message
success = delete_message_attachments(message_id="abcd1234-e89b-12d3-a456-426614174000")
if success:
    print("All message attachments deleted")
```

## Error Handling

All functions in this module will raise exceptions if the database operations fail. Wrap calls in try-except blocks for error handling:

```python
try:
    attachment = get_attachment(file_id="invalid-id")
except Exception as e:
    print(f"Error accessing attachment: {str(e)}")
```

## Content Integrity

This module implements content integrity verification using SHA-256 hashing:

1. When creating an attachment, a hash of the content is stored
2. When retrieving an attachment, the hash is verified
3. A special function (`update_attachment_content`) must be used to update content and rehash

## Database Schema

Attachments are stored in the `attachments` table with the following schema:

```sql
CREATE TABLE attachments (
    file_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id   UUID NOT NULL,
    filename     VARCHAR(255) NOT NULL,
    type         VARCHAR(255) NOT NULL,  -- MIME type
    size         BIGINT NOT NULL,        -- in bytes
    content      TEXT NOT NULL,          -- file content as text
    token_count  INTEGER NOT NULL DEFAULT 0,
    content_hash VARCHAR(64) NOT NULL,   -- SHA-256 hash
    author       JSONB NOT NULL,         -- who uploaded/generated it
    purpose      VARCHAR(255) NOT NULL,  -- e.g., "reference", "embedded-image"
    metadata     JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_attachments_message
      FOREIGN KEY(message_id) 
      REFERENCES messages(message_id) 
      ON DELETE CASCADE
);
``` 