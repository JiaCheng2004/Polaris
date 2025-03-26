# Thread Database Operations

This module provides CRUD (Create, Read, Update, Delete) operations for thread management in the database.

## Overview

Threads represent conversation threads in the application. Each thread contains metadata about the conversation such as:
- The AI model being used
- The provider of the model
- Token usage statistics
- Cost tracking
- Purpose of the conversation
- Author information

## Usage

### Import

```python
# Import specific functions
from tools.database.thread import create_thread, get_thread, list_threads, update_thread, delete_thread

# Or import the entire module
import tools.database.thread as thread_db
```

### Creating a Thread

```python
# Example: Creating a new thread
author_data = {
    "user_id": "user123",
    "username": "johndoe",
    "email": "john@example.com"
}

new_thread = create_thread(
    model="gpt-4",
    provider="openai",
    purpose="web chat assistant",
    author=author_data
)

# Access the thread ID
thread_id = new_thread["thread_id"]
print(f"Created thread with ID: {thread_id}")
```

### Retrieving Threads

```python
# Get a specific thread by ID
thread = get_thread(thread_id="123e4567-e89b-12d3-a456-426614174000")

# List threads with filtering and sorting
recent_threads = list_threads(
    limit=10,
    order_by="created_at.desc",
    filters={"provider": "anthropic"}
)

# Iterate through threads
for thread in recent_threads:
    print(f"Thread {thread['thread_id']} created at {thread['created_at']}")
```

### Updating Threads

```python
# Update thread properties
updated_thread = update_thread(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    update_data={
        "model": "claude-3-opus",
        "provider": "anthropic",
        "purpose": "updated purpose"
    }
)

# Increment usage statistics
updated_thread = increment_thread_usage(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    additional_tokens=150,
    additional_cost=0.0025
)
```

### Deleting Threads

```python
# Delete a thread (and all associated messages/attachments/vectors)
success = delete_thread(thread_id="123e4567-e89b-12d3-a456-426614174000")
if success:
    print("Thread deleted successfully")
```

## Error Handling

All functions in this module will raise exceptions if the database operations fail. Wrap calls in try-except blocks for error handling:

```python
try:
    thread = get_thread(thread_id="invalid-id")
except Exception as e:
    print(f"Error accessing thread: {str(e)}")
```

## Database Schema

Threads are stored in the `threads` table with the following schema:

```sql
CREATE TABLE threads (
    thread_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model        VARCHAR(255) NOT NULL,
    provider     VARCHAR(255) NOT NULL,
    tokens_spent BIGINT NOT NULL DEFAULT 0,
    cost         DECIMAL(12, 2) NOT NULL DEFAULT 0.00,
    purpose      VARCHAR(255) NOT NULL,
    author       JSONB NOT NULL,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
``` 