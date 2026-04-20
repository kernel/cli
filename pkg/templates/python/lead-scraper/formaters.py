import json
import re
from typing import Any, Iterable
from models import BusinessLead

_JSON_FENCE_RE = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.IGNORECASE | re.DOTALL)
_TRAILING_COMMA_RE = re.compile(r",\s*([\]}])")
_SMART_QUOTES = {
    "\u201c": '"', "\u201d": '"',  # “ ”
    "\u2018": "'", "\u2019": "'",  # ‘ ’
}


def parse_leads_from_result(result_text: str) -> list[BusinessLead]:
    """
    Robustly extract a JSON array of leads from an LLM/browser agent output and
    convert it into BusinessLead objects.

    Strategy:
      1) Prefer JSON inside ```json ... ``` fenced blocks
      2) Else try to decode from the first '[' onwards using JSONDecoder.raw_decode
      3) Normalize a few common LLM issues (smart quotes, trailing commas, "null" strings)
    """
    if not result_text or not result_text.strip():
        return []

    candidates = _extract_json_candidates(result_text)

    for candidate in candidates:
        parsed = _try_parse_json_list(candidate)
        if parsed is None:
            continue

        leads: list[BusinessLead] = []
        for raw in parsed:
            lead = _to_business_lead(raw)
            if lead is not None:
                leads.append(lead)

        if leads:
            return leads  # first successful parse wins

    # Fallback: try to parse markdown format (when agent returns numbered lists)
    leads = _parse_markdown_leads(result_text)
    if leads:
        return leads

    return []


def _parse_markdown_leads(text: str) -> list[BusinessLead]:
    """
    Parse markdown-formatted lead data when JSON parsing fails.
    Handles format like:
    1. **Business Name**
       - Address: 123 Main St
       - Rating: 4.5
       - Phone: +1 555-1234
    """
    leads = []
    
    # Pattern to match numbered entries with bold names
    entry_pattern = re.compile(
        r'\d+\.\s*\*\*(.+?)\*\*\s*\n((?:\s*-\s*.+\n?)+)',
        re.MULTILINE
    )
    
    for match in entry_pattern.finditer(text):
        name = match.group(1).strip()
        details = match.group(2)
        
        # Extract fields from the dash-prefixed lines
        def extract_field(pattern: str, txt: str) -> str | None:
            m = re.search(pattern, txt, re.IGNORECASE)
            return m.group(1).strip() if m else None
        
        address = extract_field(r'-\s*Address:\s*(.+?)(?:\n|$)', details)
        rating_str = extract_field(r'-\s*Rating:\s*([\d.]+)', details)
        review_str = extract_field(r'-\s*Review\s*Count:\s*([\d,]+)', details)
        category = extract_field(r'-\s*Category:\s*(.+?)(?:\n|$)', details)
        phone = extract_field(r'-\s*Phone:\s*(.+?)(?:\n|$)', details)
        website = extract_field(r'-\s*Website:\s*(.+?)(?:\n|$)', details)
        
        # Clean up "Not available" etc
        if phone and phone.lower() in ('not available', 'n/a', 'none'):
            phone = None
        if website and website.lower() in ('not available', 'n/a', 'none'):
            website = None
        
        try:
            lead = BusinessLead(
                name=name,
                address=address,
                rating=float(rating_str) if rating_str else None,
                review_count=int(review_str.replace(',', '')) if review_str else None,
                category=category,
                phone=phone,
                website=website,
            )
            leads.append(lead)
        except Exception:
            continue
    
    return leads


def _extract_json_candidates(text: str) -> list[str]:
    """
    Return possible JSON snippets, ordered from most to least likely.
    """
    # 1) Fenced code blocks first
    fenced = [m.group(1) for m in _JSON_FENCE_RE.finditer(text)]
    if fenced:
        return fenced

    # 2) Otherwise try from first '[' onward (common "Return ONLY a JSON array")
    idx = text.find("[")
    return [text[idx:]] if idx != -1 else []


def _normalize_llm_json(s: str) -> str:
    # Replace smart quotes
    for k, v in _SMART_QUOTES.items():
        s = s.replace(k, v)

    # Some models do ``key``: ``value``. Convert double-backticks to quotes carefully.
    # (Keep this minimal: it can still be wrong, but it helps common cases.)
    s = s.replace("``", '"')

    # Convert string "null" to JSON null
    s = s.replace('"null"', "null")

    # Remove trailing commas before ] or }
    s = _TRAILING_COMMA_RE.sub(r"\1", s)

    return s.strip()


def _try_parse_json_list(candidate: str) -> list[dict[str, Any]] | None:
    """
    Attempt to parse a JSON array from a candidate snippet.
    Returns a list of dicts or None.
    """
    candidate = _normalize_llm_json(candidate)

    # 1) Direct parse
    try:
        data = json.loads(candidate)
        return data if isinstance(data, list) else None
    except json.JSONDecodeError:
        pass

    # 2) Decoder-based parse from first '[' (more robust than find/rfind slicing)
    start = candidate.find("[")
    if start == -1:
        return None

    decoder = json.JSONDecoder()
    try:
        obj, _end = decoder.raw_decode(candidate[start:])
        return obj if isinstance(obj, list) else None
    except json.JSONDecodeError:
        return None


def _to_business_lead(raw: Any) -> BusinessLead | None:
    """
    Convert one raw object into a BusinessLead, best-effort.
    """
    if not isinstance(raw, dict):
        return None

    try:
        # Optionally coerce some common fields
        rating = raw.get("rating")
        if isinstance(rating, str):
            rating = _safe_float(rating)

        review_count = raw.get("review_count")
        if isinstance(review_count, str):
            review_count = _safe_int(review_count)

        return BusinessLead(
            name=(raw.get("name") or "Unknown").strip() if isinstance(raw.get("name"), str) else (raw.get("name") or "Unknown"),
            phone=raw.get("phone"),
            address=raw.get("address"),
            website=raw.get("website"),
            rating=rating,
            review_count=review_count,
            category=raw.get("category"),
        )
    except Exception:
        # Keep parsing the rest; caller decides how to log
        return None


def _safe_float(x: str) -> float | None:
    try:
        return float(x.replace(",", "").strip())
    except Exception:
        return None


def _safe_int(x: str) -> int | None:
    try:
        return int(x.replace(",", "").strip())
    except Exception:
        return None
