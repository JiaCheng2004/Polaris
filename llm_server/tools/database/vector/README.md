# Vector Database Operations

This module provides CRUD (Create, Read, Update, Delete) operations for vector embeddings management in the database.

## Overview

Vectors represent text embeddings stored for semantic search. Each vector contains:
- The embedding array
- The original text content that was embedded
- References to thread and/or message IDs
- Namespace and metadata

## Usage

### Import

```python
# Import specific functions
from tools.database.vector import create_vector, get_vector, list_vectors, search_vectors, update_vector, delete_vector

# Or import the entire module
import tools.database.vector as vector_db
```

### Creating a Vector

```python
# Example: Creating a new vector embedding
embedding = [0.1, 0.2, 0.3, 0.4, ...]  # Vector from your embedding model

new_vector = create_vector(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    message_id="abcd1234-e89b-12d3-a456-426614174000",  # Optional
    embedding=embedding,
    content="This is the text that was embedded",
    metadata={"source": "message_content", "chunk_id": 1},
    namespace="my_namespace"
)

# Access the vector ID
vector_id = new_vector["vector_id"]
print(f"Created vector with ID: {vector_id}")
```

### Retrieving Vectors

```python
# Get a specific vector by ID
vector = get_vector(vector_id="567e4567-e89b-12d3-a456-426614174000")

# List vectors with filtering
thread_vectors = list_vectors(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    namespace="my_namespace",
    limit=50
)

# Iterate through vectors
for vector in thread_vectors:
    print(f"Vector {vector['vector_id']} - Content: {vector['content'][:30]}...")
```

### Semantic Search

```python
# Search for similar vectors using a query embedding
query_embedding = [0.15, 0.25, 0.35, 0.45, ...]  # Vector from your embedding model

search_results = search_vectors(
    query_embedding=query_embedding,
    namespace="my_namespace",
    thread_id="123e4567-e89b-12d3-a456-426614174000",  # Optional filter
    similarity_threshold=0.8,  # Minimum similarity score (0-1)
    limit=5  # Return top 5 results
)

# Process search results
for result in search_results:
    similarity = result["similarity"]
    content = result["content"]
    print(f"Similarity: {similarity:.2f} - Content: {content[:50]}...")
```

### Updating Vectors

```python
# Update vector data
updated_vector = update_vector(
    vector_id="567e4567-e89b-12d3-a456-426614174000",
    update_data={
        "metadata": {"source": "message_content", "chunk_id": 2, "updated": True},
        "namespace": "updated_namespace"
    }
)

# Update the embedding itself
new_embedding = [0.11, 0.22, 0.33, 0.44, ...]
updated_vector = update_vector(
    vector_id="567e4567-e89b-12d3-a456-426614174000",
    update_data={
        "embedding": new_embedding,
        "content": "Updated text content that was re-embedded"
    }
)
```

### Deleting Vectors

```python
# Delete a specific vector
success = delete_vector(vector_id="567e4567-e89b-12d3-a456-426614174000")
if success:
    print("Vector deleted successfully")

# Delete all vectors for a thread
success = delete_thread_vectors(
    thread_id="123e4567-e89b-12d3-a456-426614174000",
    namespace="my_namespace"  # Optional filter
)
if success:
    print("All thread vectors deleted")

# Delete all vectors for a message
success = delete_message_vectors(message_id="abcd1234-e89b-12d3-a456-426614174000")
if success:
    print("All message vectors deleted")
```

## Error Handling

All functions in this module will raise exceptions if the database operations fail. Wrap calls in try-except blocks for error handling:

```python
try:
    vector = get_vector(vector_id="invalid-id")
except Exception as e:
    print(f"Error accessing vector: {str(e)}")
```

## Vector Search Capabilities

This module leverages the pgvector extension in PostgreSQL for efficient vector similarity search. The `search_vectors` function uses a stored PostgreSQL function that performs cosine similarity calculations.

## Database Schema

Vectors are stored in the `vectors` table with the following schema:

```sql
CREATE TABLE vectors (
    vector_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id    UUID NOT NULL,
    message_id   UUID,
    embedding    vector(1536) NOT NULL,  -- Using pgvector extension
    content      TEXT NOT NULL,          -- The text that was embedded
    metadata     JSONB NOT NULL DEFAULT '{}',
    namespace    VARCHAR(255) NOT NULL DEFAULT 'default',
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_vectors_thread
      FOREIGN KEY(thread_id) 
      REFERENCES threads(thread_id) 
      ON DELETE CASCADE,
      
    CONSTRAINT fk_vectors_message
      FOREIGN KEY(message_id) 
      REFERENCES messages(message_id) 
      ON DELETE CASCADE
);

CREATE INDEX ON vectors USING ivfflat (embedding vector_cosine_ops)
  WITH (lists = 100);
``` 