1. **Navigate to the main folder**:
   ```bash
   cd Polaris
   ```

   - Make sure to modify `example.config.json` located at `llm_server/config/` folder with your own API Keys

2. **Build and run both services**:
   ```bash
   docker compose up --build
   ```
   - This will spin up two containers:  
     - **llm_server** (the C++ LLM server on port 8080)  
     - **bot** (currently just a placeholder container).  

3. **Verify** the LLM server is up:
   - You can open `http://localhost:8080` (or use `curl`) to check if your LLM server endpoint is running.  

4. **Run only the LLM server** (optional):
   ```bash
   docker compose up --build llm_server
   ```
   - This command will ignore the `bot` service entirely and run only the LLM server container.

5. **Shut down** when you’re done:
   ```bash
   docker compose down
   ```
