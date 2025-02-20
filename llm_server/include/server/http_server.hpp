#ifndef HTTP_SERVER_HPP
#define HTTP_SERVER_HPP

namespace server
{
/**
 * @brief Starts the HTTP server, listening on a specified port.
 *
 * This function typically blocks until the server is shut down. The default
 * port is 8080, but it may be configured differently in the implementation.
 */
void startServer();

} // namespace server

#endif // HTTP_SERVER_HPP
