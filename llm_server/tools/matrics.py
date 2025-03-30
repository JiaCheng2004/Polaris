# tools/matrics.py

from prometheus_client import Gauge, Counter

# Example: memory usage in bytes
MEMORY_USAGE = Gauge("app_memory_usage_bytes", "Memory usage in bytes")

# Example: custom counter
REQUEST_COUNT = Counter("app_requests_total", "Total number of requests handled by the server")

def update_memory_usage(value: float):
    MEMORY_USAGE.set(value)

def increment_request_count():
    REQUEST_COUNT.inc()

def get_current_metrics():
    """
    Returns a dictionary with the current metrics values.
    """
    return {
        "memory_usage_bytes": MEMORY_USAGE._value.get(),
        "total_requests": REQUEST_COUNT._value.get()
    }
