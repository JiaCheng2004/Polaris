# models/deepseek/deepseek_chat.py

import json
import os
import shutil
import uuid
from typing import Any, Dict, List, Optional
from tools.config.load import DEEPSEEK_API_KEY

# Database imports
from tools.database.thread.create import create_thread
from tools.database.thread.update import update_thread
from tools.database.thread.read import get_thread, list_threads
from tools.database.thread.delete import delete_thread
from tools.database.message.create import create_message
from tools.database.message.update import update_message
from tools.database.message.read import get_message, list_messages, get_thread_conversation
from tools.database.message.delete import delete_message
from tools.database.file import create_file, find_existing_file_by_hash, update_file_by_content_hash
from tools.database.vector.create import create_vector
from tools.database.vector.read import search_vectors

# Tool imports
from tools.embed.text import embed_text
from tools.parse.parser import Parse

# LangChain imports
from langchain.prompts import PromptTemplate
from langchain_core.messages import BaseMessage, HumanMessage, AIMessage, SystemMessage
from langchain.schema import Document
from langchain_deepseek import ChatDeepSeek
from langchain.chains import create_retrieval_chain
from langchain.chains.combine_documents import create_stuff_documents_chain

def _join_text_chunks(content_blocks: List[dict]) -> str:
    """
    Combine JSON content blocks (e.g. [{type: 'text', text: '...'}])
    into a single text string. Customize as needed (line breaks, spacing, etc.).
    """
    # Example: join with newlines
    text_chunks = []
    for block in content_blocks:
        # block is expected to have keys: {"type": "text", "text": "..."}
        if block.get("type") == "text":
            text_chunks.append(block.get("text", ""))
    return "\n".join(text_chunks)

def _convert_to_langchain_messages(payload_messages: List[dict]) -> List[BaseMessage]:
    """
    Convert the user-provided messages into LangChain messages for the LLM.
    Each message is a dict with:
      {
        "role": "system" | "user" | "assistant",
        "content": [ { "type":"text", "text": "..." }, ... ],
        "attachments": []
      }
    """
    converted = []
    for msg in payload_messages:
        role = msg.get("role")
        # Join the text portions into a single string for the LLM
        text_str = _join_text_chunks(msg.get("content", []))

        if role == "system":
            converted.append(SystemMessage(content=text_str))
        elif role == "assistant":
            converted.append(AIMessage(content=text_str))
        else:
            # treat everything else as user/human
            converted.append(HumanMessage(content=text_str))

    return converted

def _setup_temp_directory(thread_id: str) -> str:
    """
    Create a temporary directory for file attachments
    
    Args:
        thread_id: The ID of the thread
        
    Returns:
        str: Path to the temporary directory
    """
    temp_dir = f"/tmp/{thread_id}"
    if os.path.exists(temp_dir):
        shutil.rmtree(temp_dir)
    os.makedirs(temp_dir, exist_ok=True)
    return temp_dir

def _process_files(
    files: List[Any], 
    thread_id: str, 
    message_id: str, 
    author: Dict[str, Any]
) -> List[Dict[str, Any]]:
    """
    Process files - store locally, parse content, and create vectors
    
    Args:
        files: List of file objects
        thread_id: The ID of the thread
        message_id: The ID of the message
        author: JSON data describing who uploaded the files
    
    Returns:
        List[Dict[str, Any]]: List of processed file data
    """
    if not files:
        return []
    
    # Setup temporary directory
    temp_dir = _setup_temp_directory(thread_id)
    
    # Initialize the parser
    parser = Parse()
    file_data = []
    
    for file in files:
        # Save file to temporary directory
        file_path = os.path.join(temp_dir, file.filename)
        with open(file_path, "wb") as f:
            f.write(file.read())
        
        # Parse file content
        parse_result = parser.parse(file_path)
        
        if parse_result["status"] == 200:
            content = parse_result["content"]
            parse_tool = {"type": "parser", "tools": parse_result["tools_used"]}
            file_size = os.path.getsize(file_path)
            file_type = file.content_type
            
            # Create file in database
            file_record = create_file(
                message_id=message_id,
                filename=file.filename,
                file_type=file_type,
                size=file_size,
                content=content,
                author=author,
                purpose="reference",
                token_count=0,  # Could calculate token count if needed
                metadata={},
                parse_tool=parse_tool
            )
            
            # Generate embedding for content
            embedding = embed_text(content)
            
            if embedding:
                # Create vector in database for retrieval
                vector = create_vector(
                    thread_id=thread_id,
                    message_id=message_id,
                    embedding=embedding,
                    content=content,
                    metadata={"filename": file.filename, "file_type": file_type},
                    namespace="files",
                    embed_tool={"type": "gemini", "model": "gemini-embedding-exp-03-07"}
                )
                
                file_data.append({
                    "file": file_record,
                    "vector": vector
                })
    
    return file_data

def _retrieve_relevant_contexts(thread_id: str, query: str) -> List[str]:
    """
    Retrieve relevant contexts from the vector store
    
    Args:
        thread_id: The ID of the thread
        query: The query to search for
        
    Returns:
        List[str]: List of relevant contexts
    """
    # Generate embedding for query
    query_embedding = embed_text(query)
    
    if not query_embedding:
        return []
    
    # Search for similar vectors
    similar_vectors = search_vectors(
        query_embedding=query_embedding,
        namespace="files",
        thread_id=thread_id,
        similarity_threshold=0.7,
        limit=5
    )
    
    # Extract content from similar vectors
    contexts = [vector.get("content", "") for vector in similar_vectors]
    return contexts

def create_deepseek_chat_completion(payload: dict, files: List[Any]) -> dict:
    """
    Main function with RAG capabilities to handle chat completions with DeepSeek.
    
    Implements:
    1) Thread management (create new thread or use existing)
    2) Process files and store in vector database
    3) Retrieval of relevant documents for context
    4) LLM RAG-based response generation
    
    Args:
        payload: The parsed JSON payload
        files: List of uploaded files
        
    Returns:
        Dict containing model response and metadata
    """
    # Extract fields from payload
    provider = payload.get("provider", "deepseek")
    model_name = payload.get("model", "deepseek-chat")
    messages = payload.get("messages", [])
    thread_id = payload.get("thread_id")
    purpose = payload.get("purpose", "chat")
    
    # Define default author
    author = payload.get("author", {"name": "user", "id": "anonymous"})
    
    # Check if thread exists or create new one
    existing_thread = None
    if thread_id:
        try:
            existing_thread = get_thread(thread_id)
        except Exception:
            existing_thread = None
    
    if not existing_thread:
        # Create new thread
        thread_data = create_thread(
            model=model_name,
            provider=provider,
            purpose=purpose,
            author=author,
            tokens_spent=0
        )
        thread_id = thread_data[0]["thread_id"]
    else:
        thread_id = existing_thread["thread_id"]
    
    # Store incoming messages
    stored_messages = []
    for msg in messages:
        # Convert list of content blocks to proper JSON structure
        content_blocks = msg.get("content", [])
        content_json = {"blocks": content_blocks}
        
        msg_data = create_message(
            thread_id=thread_id,
            role=msg.get("role", "user"),
            content=content_json,
            purpose="chat",
            author=author
        )
        stored_messages.append(msg_data)
    
    # Process the last message (assumed to be the user's query)
    latest_message = stored_messages[-1] if stored_messages else None
    latest_message_id = latest_message["message_id"] if latest_message else None
    
    # Process files if any and store in vector store
    if files and latest_message_id:
        file_data = _process_files(
            files=files,
            thread_id=thread_id,
            message_id=latest_message_id,
            author=author
        )
    
    # Get user query from the latest message
    user_query = _join_text_chunks(messages[-1].get("content", [])) if messages else ""
    
    # Setup DeepSeek LLM 
    llm = ChatDeepSeek(
        model=model_name,
        temperature=0.2,
        max_tokens=None,
        timeout=None,
        max_retries=2,
        api_key=DEEPSEEK_API_KEY
    )
    
    # Retrieve relevant contexts if any
    contexts = _retrieve_relevant_contexts(thread_id, user_query)
    
    if contexts:
        # Create RAG chain with retrieved contexts
        context_str = "\n\n".join(contexts)
        
        # Prompt template for answering with context
        rag_prompt = PromptTemplate.from_template(
            """You are a helpful assistant that provides accurate information based on the context given.
            
            Context:
            {context}
            
            Question: {question}
            
            Answer the question based on the context provided. If the context doesn't contain relevant information,
            use your general knowledge but clearly indicate when you're doing so.
            """
        )
        
        # Create documents from contexts
        docs = [Document(page_content=context) for context in contexts]
        
        # Create answer chain
        answer_chain = create_stuff_documents_chain(llm, rag_prompt)
        
        # Generate answer
        answer = answer_chain.invoke({
            "question": user_query,
            "context": docs
        })
        
        response_content = answer["answer"]
    else:
        # No context available, use regular conversation flow
        conversation_msgs = _convert_to_langchain_messages(messages)
        ai_message = llm.invoke(conversation_msgs)
        response_content = ai_message.content
    
    # Store the assistant response
    assistant_content_blocks = [{"type": "text", "text": response_content}]
    assistant_content = {"blocks": assistant_content_blocks}
    
    created_msg = create_message(
        thread_id=thread_id,
        role="assistant",
        content=assistant_content,
        purpose="chat",
        author={"name": "assistant", "id": model_name}
    )
    
    # Return response with thread info
    return {
        "thread_id": thread_id,
        "assistant_message": created_msg,
        "context_used": bool(contexts)
    }
