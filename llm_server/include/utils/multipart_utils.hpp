#ifndef MULTIPART_UTILS_HPP
#define MULTIPART_UTILS_HPP

#include <crow/multipart.h>
#include <string>

namespace utils
{
    /**
     * Extracts the form-data field name ("files", "json", etc.) 
     * from the "Content-Disposition" header of a Crow multipart part.
     *
     * E.g. if the header is: 
     *      Content-Disposition: form-data; name="files"; filename="my.jpg"
     * then the returned string is "files".
     *
     * Returns empty string if "name" is not found.
     */
    std::string getPartName(const crow::multipart::part& part);

} // namespace utils

#endif // MULTIPART_UTILS_HPP
