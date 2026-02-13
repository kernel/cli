/**
 * Helper utilities for response parsing and URL normalization
 * 
 * Note: Page manipulation (scrolling, popup dismissal) is now handled
 * by Anthropic Computer Use in qa-computer-use.ts
 */

// ============================================================================
// Response Parsing Helpers
// ============================================================================


/**
 * Parse AI response to extract JSON array.
 * Handles cases where the response contains additional text around the JSON.
 */
export function parseAIResponse<T>(response: string): T[] {
  try {
    // Try to find JSON array in the response
    const jsonMatch = response.match(/\[[\s\S]*\]/);
    if (jsonMatch) {
      const parsed = JSON.parse(jsonMatch[0]);
      if (Array.isArray(parsed)) {
        return parsed;
      }
      console.warn("Parsed JSON is not an array:", parsed);
      return [];
    }
    
    // If no array found, log the response for debugging
    console.warn("No JSON array found in AI response. Response preview:", response.substring(0, 500));
    return [];
  } catch (error) {
    console.error("Error parsing AI response:", error);
    console.error("Response that failed to parse:", response.substring(0, 1000));
    return [];
  }
}

// ============================================================================
// HTML Escaping
// ============================================================================

const HTML_ESCAPE_MAP: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#039;",
};

/**
 * Escape HTML special characters to prevent XSS in generated reports.
 */
export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (char) => HTML_ESCAPE_MAP[char] || char);
}

// ============================================================================
// URL Utilities
// ============================================================================

/**
 * Normalize and validate a URL.
 * Adds https:// prefix if missing and validates the URL format.
 *
 * @throws Error if the URL is invalid
 */
export function normalizeUrl(url: string): string {
  let normalizedUrl = url;

  if (!normalizedUrl.startsWith("http://") && !normalizedUrl.startsWith("https://")) {
    normalizedUrl = `https://${normalizedUrl}`;
  }

  // Validate URL
  try {
    new URL(normalizedUrl);
  } catch {
    throw new Error(`Invalid URL: ${url}`);
  }

  return normalizedUrl;
}
