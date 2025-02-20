#ifndef MULTIPART_UTILS_HPP
#define MULTIPART_UTILS_HPP

#include <string>

namespace utils
{
/**
 * @brief Represents a single part of a multipart/form-data submission.
 *
 * Contains a filename, the entire file body in memory, and the content type.
 */
struct MultipartPart
{
    std::string filename;    ///< e.g. "my_image.png"
    std::string body;        ///< Raw file content
    std::string contentType; ///< e.g. "image/png"
};

/**
 * @brief Extracts the 'name' attribute from a multipart form-data header.
 *
 * For example, if the Content-Disposition header is:
 *     Content-Disposition: form-data; name="files"; filename="my.jpg"
 * the returned string would be "files".
 *
 * @param part The multipart part from which to extract the field name.
 * @return The form-data field name (e.g., "files"), or an empty string if not found.
 */
std::string getPartName(const MultipartPart &part);

} // namespace utils

#endif // MULTIPART_UTILS_HPP
