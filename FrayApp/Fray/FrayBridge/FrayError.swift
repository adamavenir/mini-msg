import Foundation

/// Errors from the Fray FFI bridge
enum FrayError: LocalizedError {
    case notConnected
    case connectionFailed
    case ffi(String)
    case decodingFailed(String)
    case invalidCursor(String)

    var errorDescription: String? {
        switch self {
        case .notConnected:
            return "Not connected to a Fray project"
        case .connectionFailed:
            return "Failed to connect to Fray database"
        case .ffi(let message):
            return "FFI error: \(message)"
        case .decodingFailed(let message):
            return "Failed to decode response: \(message)"
        case .invalidCursor(let cursor):
            return "Invalid cursor format: \(cursor)"
        }
    }
}
