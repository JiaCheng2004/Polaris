# Message Database Operations

This module provides CRUD (Create, Read, Update, Delete) operations for message management in the database.

## Overview

Messages represent individual interactions in a conversation thread. Each message contains:
- The role of the sender (user, assistant, system)
- Content in JSON format for flexible representation
- Purpose information
- Author details

## Usage

### Import

```python
# Import specific functions
from tools.database.message import create_message, get_message, list_messages, update_message, delete_message

# Or import the entire module
import tools.database.message as message_db
```

### Creating a Message

```python
# Example: Creating a new message
author_data = {
    "user_id": "user123",
    "username": "johndoe"
}

# Text content example
content_data = {
    "type": "text",
    "text": "How can I help you today?"
}

# Create the message
new_message = create_message(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    role="assistant",
    content=content_data,
    purpose="reply",
    author=author_data
)

# Access the message ID
message_id = new_message["message_id"]
print(f"Created message with ID: {message_id}")
```

### Retrieving Messages

```python
# Get a specific message by ID
message = get_message(message_id="abcd1234-e89b-12d3-a456-426614174000")

# List messages with filtering
thread_messages = list_messages(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    limit=50,
    role="user"
)

# Get all messages in a thread as a conversation
conversation = get_thread_conversation(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    include_system=False,
    newest_first=False
)

# Iterate through messages
for message in conversation:
    print(f"[{message['role']}]: {message['content']}")
```

### Updating Messages

```python
# Update message content
updated_message = update_message(
    message_id="abcd1234-e89b-12d3-a456-426614174000",
    update_data={
        "content": {
            "type": "text",
            "text": "Updated message content"
        }
    }
)
```

### Deleting Messages

```python
# Delete a specific message
success = delete_message(message_id="abcd1234-e89b-12d3-a456-426614174000")
if success:
    print("Message deleted successfully")

# Delete all messages in a thread
success = delete_thread_messages(thread_id="123e4567-e89b-12d3-a456-426614174000")
if success:
    print("All thread messages deleted")
```

## Error Handling

All functions in this module will raise exceptions if the database operations fail. Wrap calls in try-except blocks for error handling:

```python
try:
    message = get_message(message_id="invalid-id")
except Exception as e:
    print(f"Error accessing message: {str(e)}")
```

## Database Schema

Messages are stored in the `messages` table with the following schema:

```sql
CREATE TABLE messages (
    message_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id   UUID NOT NULL,
    role        VARCHAR(50) NOT NULL,   -- e.g. user | assistant | system
    content     JSONB NOT NULL,         -- flexible JSON for text blocks
    purpose     VARCHAR(255) NOT NULL,  -- e.g., "reply", "summary", "annotation"
    author      JSONB NOT NULL,         -- JSON representing who/what authored the message
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_messages_thread
      FOREIGN KEY(thread_id) 
      REFERENCES threads(thread_id) 
      ON DELETE CASCADE
);
``` 