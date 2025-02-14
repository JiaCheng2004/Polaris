// app/llm_server/include/server/http_server.hpp

#ifndef HTTP_SERVER_HPP
#define HTTP_SERVER_HPP

namespace server {
    /**
     * Starts the HTTP server (e.g., with Crow or another framework).
     * It should listen on port 8080 (by default) and route requests
     * to the appropriate request handler.
     */
    void startServer();
}

#endif // HTTP_SERVER_HPP

