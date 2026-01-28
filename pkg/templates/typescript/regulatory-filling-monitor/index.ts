import { Kernel, type KernelContext } from '@onkernel/sdk';
import { samplingLoop } from './loop';
import { KernelBrowserSession } from './session';

// ============================================================================
// Types
// ============================================================================

interface SecEdgarInput {
  company: string;
  state?: string;
  date?: string;
  record_replay?: boolean;
}

interface SecEdgarFiling {
  formAndFile: string;
  filedDate: string;
  filingEntityPerson: string;
  cik: string;
  located: string;
  fileNumber: string;
  filmNumber: string;
}

interface SecEdgarOutput {
  filings: SecEdgarFiling[];
  replay_url?: string;
}

// ============================================================================
// Constants
// ============================================================================

const SEC_EDGAR_SEARCH_URL = 'https://www.sec.gov/edgar/search/';

const FILINGS_DATA_START_MARKER = 'FILINGS_DATA:';
const FILINGS_DATA_END_MARKER = 'END_FILINGS_DATA';

// ============================================================================
// Environment
// ============================================================================

const ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;

if (!ANTHROPIC_API_KEY) {
  throw new Error('ANTHROPIC_API_KEY is not set');
}

// ============================================================================
// Prompt Builder
// ============================================================================

function buildTaskPrompt(company: string, state: string, date: string): string {
  const stateStep = state
    ? `6. Find the "Principal executive offices in" dropdown and select the state "${state}" (or its full U.S. state name)`
    : '6. Skip state filtering if not needed';

  const taskDescription = [
    `search for SEC filings for company "${company}"`,
    state ? `located in ${state}` : null,
    date ? `filed on ${date}` : null,
  ].filter(Boolean).join(' ');

  return `You are searching the SEC EDGAR database for regulatory filings.

TASK: Navigate to ${SEC_EDGAR_SEARCH_URL} and ${taskDescription}.

STEPS:
1. First, use ctrl+l to focus the URL bar, then navigate to the SEC EDGAR search page: ${SEC_EDGAR_SEARCH_URL}
2. Wait for the page to load completely
3. In the main search field (labeled "Company name, ticker, or CIK number" or similar), type: ${company}
4. Click on "Show more search options" or similar to expand the search filters
5. Set the "Filed date range" to "Custom" and enter both the from and to dates as ${date}
${stateStep}
7. Click the Search button to execute the search
8. Wait for results to load
9. Before extracting data, click on "Edit columns" or the column settings button to customize visible columns. Make sure to enable/select these columns: "CIK", "File number", "Film number". Apply the changes.
10. Extract ALL filing results from the results table. For each filing, capture:
   - formAndFile: The form type and file description (e.g., "10-K", "8-K", "4", "DEF 14A")
   - filedDate: The date filed (YYYY-MM-DD format)
   - filingEntityPerson: The company or person who filed
   - cik: The CIK number
   - located: The state/office location
   - fileNumber: The file number
   - filmNumber: The film number

IMPORTANT:
- If a popup, survey, or feedback modal appears, close it by clicking the X button or "No thanks" before continuing
- After the search completes, note the TOTAL NUMBER OF RESULTS shown (e.g., "96 search results"). Use this count to verify you have extracted ALL filings.
- Before extracting data, zoom out the page to see more results at once. Press ctrl+minus once, then press ctrl+minus again, then press ctrl+minus a third time (each as a separate key action). This makes it easier to capture all entries.
- ALWAYS use the scroll action (with scroll_direction and scroll_amount) to navigate through results. DO NOT use Page_Down, Page_Up, Home, End, or arrow keys for scrolling - these jump too far and will cause you to miss entries.
- ALWAYS set scroll_amount: 1 for each scroll action to move through the table one row at a time. Never use scroll_amount > 1.
- Keep track of how many filings you have extracted vs. the total count shown
- If no results are found, report that clearly

OUTPUT FORMAT (VERY IMPORTANT - follow exactly):
When you have finished extracting all the data, output the results as a valid JSON array. 
Start with ${FILINGS_DATA_START_MARKER} on its own line, then output a JSON array of objects, then ${FILINGS_DATA_END_MARKER} on its own line.

${FILINGS_DATA_START_MARKER}
[
  {"formAndFile": "10-K", "filedDate": "2025-01-15", "filingEntityPerson": "Company Name Inc", "cik": "0001234567", "located": "CA", "fileNumber": "001-12345", "filmNumber": "25123456"},
  {"formAndFile": "8-K", "filedDate": "2025-01-15", "filingEntityPerson": "Another Company", "cik": "0009876543", "located": "NY", "fileNumber": "001-98765", "filmNumber": "25654321"}
]
${FILINGS_DATA_END_MARKER}

Rules for the JSON:
- Output must be a valid JSON array (starts with [ and ends with ])
- Each filing is a JSON object with these exact keys: formAndFile, filedDate, filingEntityPerson, cik, located, fileNumber, filmNumber
- Use empty string "" for missing values, not null
- Dates must be in YYYY-MM-DD format
- Make sure to include ALL filings (check against the total count shown)
- ALWAYS include ${FILINGS_DATA_END_MARKER} after the JSON array

After the JSON, provide a brief summary of what you found.`;
}

// ============================================================================
// Parsing Utilities
// ============================================================================

function isValidFiling(filing: unknown): filing is SecEdgarFiling {
  if (typeof filing !== 'object' || filing === null) return false;
  const f = filing as Record<string, unknown>;
  return typeof f.formAndFile === 'string' && typeof f.filedDate === 'string';
}

function parseFilingsFromMarkers(content: string): SecEdgarFiling[] | null {
  const startIndex = content.indexOf(FILINGS_DATA_START_MARKER);
  const endIndex = content.indexOf(FILINGS_DATA_END_MARKER);

  if (startIndex === -1 || endIndex === -1) return null;

  const jsonSection = content.slice(startIndex + FILINGS_DATA_START_MARKER.length, endIndex).trim();

  // Try parsing as JSON array
  try {
    const parsed = JSON.parse(jsonSection);
    if (Array.isArray(parsed)) {
      const validFilings = parsed.filter(isValidFiling);
      console.log(`Parsed ${validFilings.length} filings from JSON array`);
      return validFilings;
    }
  } catch {
    console.log('Failed to parse as JSON array, trying line-by-line');
  }

  // Fallback: parse line by line
  const filings: SecEdgarFiling[] = [];
  for (const line of jsonSection.split('\n')) {
    const trimmedLine = line.trim().replace(/,$/, '');
    if (trimmedLine.startsWith('{') && trimmedLine.endsWith('}')) {
      try {
        const filing = JSON.parse(trimmedLine);
        if (isValidFiling(filing)) {
          filings.push(filing);
        }
      } catch {
        // Skip malformed lines
      }
    }
  }

  if (filings.length > 0) {
    console.log(`Parsed ${filings.length} filings from line-by-line`);
    return filings;
  }

  return null;
}

function parseFilingsWithRegex(content: string): SecEdgarFiling[] {
  const jsonObjectRegex = /\{[^{}]*"formAndFile"[^{}]*"filedDate"[^{}]*\}/g;
  const matches = content.match(jsonObjectRegex);

  if (!matches || matches.length === 0) {
    console.log('No filings found in result');
    return [];
  }

  const filings: SecEdgarFiling[] = [];
  for (const match of matches) {
    try {
      const filing = JSON.parse(match);
      if (isValidFiling(filing)) {
        filings.push(filing);
      }
    } catch {
      // Skip malformed objects
    }
  }

  console.log(`Parsed ${filings.length} filings using regex fallback`);
  return filings;
}

function parseFilingsFromResult(result: string): SecEdgarFiling[] {
  return parseFilingsFromMarkers(result) ?? parseFilingsWithRegex(result);
}

function extractTextFromMessages(messages: { content: string | { type: string; text?: string }[] }[]): string {
  const lastMessage = messages[messages.length - 1];
  if (!lastMessage) {
    throw new Error('Failed to get the last message from the sampling loop');
  }

  return typeof lastMessage.content === 'string'
    ? lastMessage.content
    : lastMessage.content
        .map(block => (block.type === 'text' ? block.text : ''))
        .join('');
}

// ============================================================================
// Kernel App
// ============================================================================

const kernel = new Kernel();
const app = kernel.app('ts-regulatory-filling-monitor');

// How to invoke with Kernel:
//   kernel deploy index.ts --env-file .env
//   kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "Apple"}'
//   kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "AAPL", "state": "CA"}'
//   kernel invoke ts-regulatory-filling-monitor sec-edgar-task --payload '{"company": "Tesla", "date": "2025-01-15"}'
//
// Parameters:
//   - company (required): company name, ticker symbol, or CIK number
//   - state (optional): state/office to filter, e.g. "CA", "NY"
//   - date (optional): YYYY-MM-DD; defaults to today if omitted
//   - record_replay (optional): true to record a video replay of the browser session

app.action<SecEdgarInput, SecEdgarOutput>(
  'sec-edgar-task',
  async (ctx: KernelContext, payload?: SecEdgarInput): Promise<SecEdgarOutput> => {
    if (!payload?.company) {
      throw new Error('company is required');
    }

    const company = payload.company;
    const state = payload.state ?? '';
    const date = payload.date ?? new Date().toLocaleDateString('en-CA');

    // Create browser session with optional replay recording
    const session = new KernelBrowserSession(kernel, {
      stealth: true,
      recordReplay: payload.record_replay ?? false,
    });

    await session.start();
    console.log('Kernel browser live view url:', session.liveViewUrl);

    try {
      const taskPrompt = buildTaskPrompt(company, state, date);

      const finalMessages = await samplingLoop({
        model: 'claude-sonnet-4-5-20250929',
        messages: [{ role: 'user', content: taskPrompt }],
        apiKey: ANTHROPIC_API_KEY,
        thinkingBudget: 1024,
        kernel,
        sessionId: session.sessionId,
      });

      if (finalMessages.length === 0) {
        throw new Error('No messages were generated during the sampling loop');
      }

      const result = extractTextFromMessages(finalMessages);
      const filings = parseFilingsFromResult(result);
      const sessionInfo = await session.stop();

      return {
        filings,
        replay_url: sessionInfo.replayViewUrl,
      };
    } catch (error) {
      console.error('Error in SEC EDGAR task:', error);
      await session.stop();
      throw error;
    }
  },
);
