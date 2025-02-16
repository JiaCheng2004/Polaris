// app/llm_server/include/server/http_server.hpp

#ifndef HTTP_SERVER_HPP
#define HTTP_SERVER_HPP

namespace server {

    /**
     * Starts the HTTP server, listening on port 8080 (by default).
     * This function typically blocks until the server is shut down.
     */
    void startServer();

} // namespace server

#endif // HTTP_SERVER_HPP

