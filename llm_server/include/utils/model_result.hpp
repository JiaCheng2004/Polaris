#ifndef MODEL_RESULT_HPP
#define MODEL_RESULT_HPP

#include <string>
#include <vector>

namespace models {

/**
 * A simple struct that models might return.
 */
struct ModelResult
{
    bool success = true;
    std::string modelUsed;
    std::string message;
    int tokenUsage = 0;
    int errorCode = 200; // 200 for OK
    std::string errorMessage;
    
    // If you need file IDs or other info from the model:
    std::vector<std::string> fileIds;
};

} // namespace models

#endif // MODEL_RESULT_HPP
