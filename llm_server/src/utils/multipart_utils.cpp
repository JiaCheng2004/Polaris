#include "utils/multipart_utils.hpp"

namespace utils
{
/**
 * @brief Extracts the name field (such as "files", "json", etc.) from a MultipartPart.
 *
 * @note In this example, the implementation simply returns the filename, but
 *       you may modify this function to parse the Content-Disposition header
 *       if needed.
 *
 * @param part The multipart part from which to extract the field name.
 * @return A string containing the extracted field name.
 */
std::string getPartName(const MultipartPart &part)
{
    return part.filename;
}

} // namespace utils
