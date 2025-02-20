#ifndef TOKEN_TRACKER_HPP
#define TOKEN_TRACKER_HPP

namespace utils
{
/**
 * @brief A utility class for tracking token usage across model operations.
 *
 * This class provides static methods to increment and retrieve a global
 * usage counter.
 */
class TokenTracker
{
public:
    /**
     * @brief Adds to the total token usage.
     * @param count The number of tokens to add to the cumulative total.
     */
    static void addUsage(int count);

    /**
     * @brief Retrieves the cumulative token usage.
     * @return The total number of tokens used so far.
     */
    static int getTotalUsage();

private:
    TokenTracker() = default;
};

} // namespace utils

#endif // TOKEN_TRACKER_HPP
