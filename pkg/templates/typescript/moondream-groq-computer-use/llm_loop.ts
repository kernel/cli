import { Groq } from 'groq-sdk';
import { jsonrepair } from 'jsonrepair';
import type { Kernel } from '@onkernel/sdk';
import { ComputerTool } from './tools/computer';
import {
  COORDINATE_SCALE,
  ComputerAction,
  DEFAULT_SCREEN_SIZE,
  type ScreenSize,
} from './tools/types/computer';
import type { MoondreamClient, MoondreamPoint } from './moondream';

const MODEL_NAME = 'openai/gpt-oss-120b';

const SYSTEM_PROMPT = [
  'You are a browser-automation controller. You do NOT see images.',
  'You must decide actions and call Moondream for any visual understanding.',
  'Return ONLY a single JSON object that matches the schema below.',
  'Parsing note: the client will extract the substring between the first \'{\' and last \'}\' and run jsonrepair on it.',
  'Therefore, do NOT include any extra text before or after the JSON object.',
  '',
  'Browser context:',
  '- The browser is already open. Do NOT request an open_browser action.',
  '',
  'Action policy:',
  '- Bundle multiple actions when you can (e.g., navigate -> moondream_query).',
  '- Use moondream_* actions for all visual understanding; keep queries short and specific.',
  '- Never emit moondream_query without a clear question.',
  '- Use click_at/type_text_at/scroll_at with coordinates in 0-1000 normalized scale.',
  '- If you need coordinates, call moondream_point first.',
  '- Prefer type_text_at with press_enter=true to submit searches; use key_combination mainly for shortcuts.',
  '- You may include post_wait_ms in args to wait after an action (agent handles it).',
  '- When a task asks for a link or page identity, use page_info after clicking or navigation.',
  '- If your actions did not change state, reassess with a new Moondream question rather than repeating.',
  '- If you need a specific item URL/details, open a specific item page (not a results list) and confirm it.',
  '- If a click does not change the page, try a different target or use hover_at to reveal link text/URL.',
  '- When opening an item, prefer clicking the title or image; verify you reached a detail page before returning its URL.',
  '- If list items offer separate “comments/discussion” links and “title/article” links, click the title/article link unless the task explicitly asks for comments.',
  '- On list pages with metadata/source links, click the title line (main link), not the source/domain/metadata line.',
  '- If the task includes constraints, use on-screen evidence to select a qualifying item before answering.',
  '- On list pages, identify a candidate item that matches constraints, then point to its title/image and click to open.',
  '- Do not answer until you can confirm you are on the target page type (e.g., a single-item detail page).',
  '- For “first/top result” tasks, click the topmost result item (not navigation, ads, or comments).',
  '- When returning a URL, use the most recent page_info URL from the current page.',
  '- Before final response for item-specific tasks, confirm the page type with moondream_query.',
  '- If a click does not open the item, try a different target or a double-click by setting clicks: 2. If you suspect a new tab opened, use key_combination with ctrl+tab and re-check page_info.',
  '- Use action result field state_changed to decide if a click/scroll had an effect; if false, adjust target or strategy.',
  '- If the user specifies a site to search (e.g., Wikipedia), use that site\'s search first; only switch to another search engine if the site search fails.',
  '- Never output placeholders like {{x}}, {{url}}, or <url_placeholder> in actions or final_response.',
  '- Do not ask Moondream to infer the URL or page title; use page_info for those.',
  '- If the task specifies a domain/URL, avoid leaving that domain unless the task explicitly requires it; if page_info shows an unexpected domain, go_back or navigate to the intended domain.',
  '- If the task specifies a domain, your final_response URL must include that domain.',
  '- After typing a search query, submit it (press_enter or search button). Avoid clicking unrelated suggestions or ads.',
  '- For tasks like “first/top result,” ask Moondream to point at the first item or top result and click it.',
  '- When moondream_point returns coordinates (x_norm/y_norm), use those exact numbers in click_at (x,y). Never use placeholders.',
  '- Do not navigate to URLs derived from Moondream answers. Only navigate to URLs provided by the user or confirmed via page_info.',
  '- If search results are not found after a couple of attempts, fallback to direct navigation to the most likely official page.',
  '- Moondream query quality matters. Ask short, concrete, visual questions. Avoid vague or multi-part questions.',
  '- When the task requires price or currency, verify the price on the detail page with a targeted Moondream query and return the exact text.',
  '- For dense result grids, you may use moondream_detect with objects like "product image" or "item card" and click the topmost box.',
  '- Never ask Moondream for a URL or link; only use page_info for URLs.',
  '',
  'Moondream query examples (good vs bad):',
  'GOOD: "Is there a search box on this page?"',
  'BAD: "What should I do next?"',
  'GOOD: "What is the exact price shown for the highlighted item?"',
  'BAD: "Tell me everything about this page."',
  'GOOD: "Is this a single-item detail page?"',
  'BAD: "Is this page good?"',
  'GOOD: "Which button says \\"Sign in\\"?"',
  'BAD: "Find the right thing."',
  'BAD: "What is the URL for this page?"',
  '',
  'Moondream query templates:',
  '- Presence: "Is there a <thing> on the page?"',
  '- Identification: "What is the exact text of the <thing>?"',
  '- Page type: "Is this a <list/detail/login> page?"',
  '- Verification: "Does the page show the item I just clicked?"',
  '- Result matching: "Which result shows the domain <domain>?"',
  '- If asked to use a search box, attempt a search interaction before using direct navigation; only fall back if stuck, and mention fallback in final_response.',
  '- If the user requests JSON output, ensure final_response is valid JSON that matches the requested fields.',
  '- When setting done=true, always include a non-empty final_response with concrete values (no placeholders like {{...}}).',
  '- Stop when the task is complete by setting done=true and final_response.',
  '',
  'JSON Schema:',
  '{',
  '  "$schema": "https://json-schema.org/draft/2020-12/schema",',
  '  "type": "object",',
  '  "properties": {',
  '    "actions": {',
  '      "type": "array",',
  '      "items": {',
  '        "type": "object",',
  '        "properties": {',
  '          "action": {',
  '            "type": "string",',
  '            "enum": [',
  '              "navigate",',
  '              "click_at",',
  '              "hover_at",',
  '              "type_text_at",',
  '              "scroll_document",',
  '              "scroll_at",',
  '              "go_back",',
  '              "go_forward",',
  '              "key_combination",',
  '              "drag_and_drop",',
  '              "wait",',
  '              "moondream_query",',
  '              "moondream_caption",',
  '              "moondream_point",',
  '              "moondream_detect",',
  '              "page_info",',
  '              "done",',
  '              "fail"',
  '            ]',
  '          },',
  '          "args": { "type": "object" }',
  '        },',
  '        "required": ["action", "args"],',
  '        "additionalProperties": false',
  '      }',
  '    },',
  '    "done": { "type": "boolean" },',
  '    "final_response": { "type": "string" },',
  '    "error": { "type": "string" }',
  '  },',
  '  "required": ["actions"],',
  '  "additionalProperties": false',
  '}',
  '',
  'Examples (valid JSON):',
  '{"actions":[{"action":"navigate","args":{"url":"https://example.com"}},{"action":"moondream_caption","args":{"length":"short"}}]}',
  '{"actions":[{"action":"moondream_point","args":{"object":"login button"}},{"action":"click_at","args":{"x":512,"y":412}}]}',
  '{"actions":[],"done":true,"final_response":"Logged in and reached the dashboard."}',
  '{"actions":[],"done":true,"final_response":"{\\"title\\":\\"Example Domain\\",\\"url\\":\\"https://example.com\\"}"}',
].join('\n');

export interface LlmOptions {
  maxIterations?: number;
  temperature?: number;
  maxCompletionTokens?: number;
  topP?: number;
  postActionWaitMs?: number;
  reasoningEffort?: 'low' | 'medium' | 'high' | string;
}

interface LlmAction {
  action: string;
  args: Record<string, unknown>;
}

interface StepLog {
  step: number;
  action: string;
  status: 'success' | 'failed';
  detail: string;
  output?: string;
}

export interface LlmResult {
  finalResponse: string;
  error?: string;
}

export async function runLlmAgent({
  query,
  moondream,
  kernel,
  sessionId,
  groqApiKey,
  options = {},
}: {
  query: string;
  moondream: MoondreamClient;
  kernel: Kernel;
  sessionId: string;
  groqApiKey: string;
  options?: LlmOptions;
}): Promise<LlmResult> {
  const groq = new Groq({ apiKey: groqApiKey });
  const computer = new ComputerTool(kernel, sessionId);

  const messages: Array<{ role: 'system' | 'user' | 'assistant'; content: string }> = [
    { role: 'system', content: SYSTEM_PROMPT },
    {
      role: 'user',
      content: `Task: ${query}\nReturn a JSON object with an actions array. Bundle multiple actions when sensible.`,
    },
  ];

  const logs: StepLog[] = [];
  const answers: string[] = [];
  let lastScreenshot: string | undefined;
  let lastPageUrl: string | undefined;
  let lastPointNorm: { x: number; y: number } | undefined;
  let error: string | undefined;

  const maxIterations = options.maxIterations ?? 40;

  for (let iteration = 1; iteration <= maxIterations; iteration++) {
    let raw: string;
    try {
      raw = await groqCompletion(groq, messages, options);
    } catch (error) {
      messages.push({
        role: 'user',
        content: 'Your last output was invalid. Return ONLY a JSON object that matches the schema.',
      });
      try {
        raw = await groqCompletion(groq, messages, options);
      } catch (err) {
        error = err instanceof Error ? err.message : String(err);
        raw = '{"actions":[]}';
      }
    }
    const batchPayload = parseJsonAction(raw);
    messages.push({ role: 'assistant', content: JSON.stringify(batchPayload) });

    const actions = normalizeActions(batchPayload);
    const results: Array<Record<string, unknown>> = [];
    let doneFlag = Boolean((batchPayload as Record<string, unknown>).done);
    let finalResponse = doneFlag ? String((batchPayload as Record<string, unknown>).final_response || '') : '';

    try {
      for (const actionItem of actions) {
        const action = String(actionItem.action || '').trim();
        const args = actionItem.args || {};
        if (!action) {
          results.push({ action: '', status: 'failed', detail: 'missing action' });
          continue;
        }

        if (action === 'navigate') {
          const url = String(args.url || '').trim();
          if (!url) throw new Error('navigate requires url');
          if (url.includes('{{') || url.includes('}}') || url.toLowerCase().includes('placeholder')) {
            logs.push({ step: iteration, action, status: 'failed', detail: 'navigate url is placeholder' });
            results.push({ action, status: 'failed', detail: 'navigate url is placeholder' });
            continue;
          }
          const result = await computer.executeAction(ComputerAction.NAVIGATE, { url });
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: `Navigated to ${url}` });
          results.push({ action, status: status(result.error), detail: `navigated to ${url}`, state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'click_at') {
          let x: number;
          let y: number;
          try {
            [x, y] = coerceCoords(args, computer.getScreenSize());
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            if (lastPointNorm) {
              x = lastPointNorm.x;
              y = lastPointNorm.y;
              results.push({ action, status: 'success', detail: 'used last moondream_point', used_last_point: true });
            } else {
              logs.push({ step: iteration, action, status: 'failed', detail: message });
              results.push({ action, status: 'failed', detail: message });
              continue;
            }
          }
          const result = await computer.executeAction(ComputerAction.CLICK_AT, { x, y });
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: 'Clicked at coordinates' });
          results.push({ action, status: status(result.error), detail: 'clicked', state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'hover_at') {
          let x: number;
          let y: number;
          try {
            [x, y] = coerceCoords(args, computer.getScreenSize());
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            logs.push({ step: iteration, action, status: 'failed', detail: message });
            results.push({ action, status: 'failed', detail: message });
            continue;
          }
          const result = await computer.executeAction(ComputerAction.HOVER_AT, { x, y });
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: 'Hovered at coordinates' });
          results.push({ action, status: status(result.error), detail: 'hovered', state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'type_text_at') {
          let x: number;
          let y: number;
          try {
            [x, y] = coerceCoords(args, computer.getScreenSize());
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            if (lastPointNorm) {
              x = lastPointNorm.x;
              y = lastPointNorm.y;
              results.push({ action, status: 'success', detail: 'used last moondream_point', used_last_point: true });
            } else {
              logs.push({ step: iteration, action, status: 'failed', detail: message });
              results.push({ action, status: 'failed', detail: message });
              continue;
            }
          }
          const text = args.text;
          if (text === undefined) throw new Error('type_text_at requires text');
          const result = await computer.executeAction(ComputerAction.TYPE_TEXT_AT, {
            x,
            y,
            text: String(text),
            press_enter: Boolean(args.press_enter),
            clear_before_typing: args.clear_before_typing !== false,
          });
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: 'Typed text' });
          results.push({ action, status: status(result.error), detail: 'typed', state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'scroll_document') {
          const direction = String(args.direction || 'down');
          const payload: Record<string, unknown> = { direction };
          if (args.magnitude !== undefined) payload.magnitude = Number(args.magnitude);
          const result = await computer.executeAction(ComputerAction.SCROLL_DOCUMENT, payload);
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: `Scrolled ${direction}` });
          results.push({ action, status: status(result.error), detail: `scrolled ${direction}`, state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'scroll_at') {
          let x: number;
          let y: number;
          try {
            [x, y] = coerceCoords(args, computer.getScreenSize());
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            logs.push({ step: iteration, action, status: 'failed', detail: message });
            results.push({ action, status: 'failed', detail: message });
            continue;
          }
          const direction = String(args.direction || 'down');
          const payload: Record<string, unknown> = { x, y, direction };
          if (args.magnitude !== undefined) payload.magnitude = Number(args.magnitude);
          const result = await computer.executeAction(ComputerAction.SCROLL_AT, payload);
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: `Scrolled ${direction}` });
          results.push({ action, status: status(result.error), detail: `scrolled ${direction}`, state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'go_back') {
          const result = await computer.executeAction(ComputerAction.GO_BACK, {});
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: 'Went back' });
          results.push({ action, status: status(result.error), detail: 'went back', state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'go_forward') {
          const result = await computer.executeAction(ComputerAction.GO_FORWARD, {});
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: 'Went forward' });
          results.push({ action, status: status(result.error), detail: 'went forward', state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'key_combination') {
          const keys = String(args.keys || '').trim();
          if (!keys) throw new Error('key_combination requires keys');
          const result = await computer.executeAction(ComputerAction.KEY_COMBINATION, { keys });
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: `Pressed ${keys}` });
          results.push({ action, status: status(result.error), detail: `pressed ${keys}`, state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'wait') {
          const seconds = Number(args.seconds ?? 1);
          await sleep(seconds * 1000);
          logs.push({ step: iteration, action, status: 'success', detail: `Waited ${seconds.toFixed(2)}s` });
          results.push({ action, status: 'success', detail: `waited ${seconds.toFixed(2)}s` });
        } else if (action === 'moondream_query') {
          const question = String(args.question || '').trim();
          if (!question) {
            logs.push({ step: iteration, action, status: 'failed', detail: 'Missing question' });
            results.push({ action, status: 'failed', detail: 'missing question' });
            continue;
          }
          const screenshot = await ensureScreenshot(computer, lastScreenshot);
          lastScreenshot = screenshot;
          const answer = await moondream.query(screenshot, question);
          answers.push(answer);
          logs.push({ step: iteration, action, status: 'success', detail: 'Answered question', output: answer });
          results.push({ action, status: 'success', answer });
        } else if (action === 'moondream_caption') {
          const length = String(args.length || 'normal') as 'short' | 'normal' | 'long';
          const screenshot = await ensureScreenshot(computer, lastScreenshot);
          lastScreenshot = screenshot;
          const caption = await moondream.caption(screenshot, length);
          answers.push(caption);
          logs.push({ step: iteration, action, status: 'success', detail: 'Captioned image', output: caption });
          results.push({ action, status: 'success', caption });
        } else if (action === 'drag_and_drop') {
          if (args.x === undefined || args.y === undefined) throw new Error('drag_and_drop requires x and y');
          if (args.destination_x === undefined || args.destination_y === undefined) {
            throw new Error('drag_and_drop requires destination_x and destination_y');
          }
          let x: number;
          let y: number;
          let destination_x: number;
          let destination_y: number;
          try {
            [x, y] = coerceCoords({ x: args.x, y: args.y }, computer.getScreenSize());
            [destination_x, destination_y] = coerceCoords(
              { x: args.destination_x, y: args.destination_y },
              computer.getScreenSize(),
            );
          } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            logs.push({ step: iteration, action, status: 'failed', detail: message });
            results.push({ action, status: 'failed', detail: message });
            continue;
          }
          const result = await computer.executeAction(ComputerAction.DRAG_AND_DROP, {
            x,
            y,
            destination_x,
            destination_y,
          });
          const { screenshot, stateChanged } = updateScreenshotWithState(result.base64Image, lastScreenshot);
          lastScreenshot = screenshot;
          logs.push({ step: iteration, action, status: status(result.error), detail: 'Dragged element' });
          results.push({ action, status: status(result.error), detail: 'dragged', state_changed: stateChanged });
          await postWait(action, args, options);
        } else if (action === 'moondream_point') {
          const objectLabel = String(args.object || '').trim();
          if (!objectLabel) throw new Error('moondream_point requires object');
          const screenshot = await ensureScreenshot(computer, lastScreenshot);
          lastScreenshot = screenshot;
          const point = await moondream.point(screenshot, objectLabel);
          if (!point) {
            logs.push({ step: iteration, action, status: 'failed', detail: 'No point found' });
            results.push({ action, status: 'failed', detail: 'no point found' });
          } else {
            const payload = pointPayload(point, computer.getScreenSize());
            if (typeof payload.x_norm === 'number' && typeof payload.y_norm === 'number') {
              lastPointNorm = { x: payload.x_norm, y: payload.y_norm };
            }
            logs.push({ step: iteration, action, status: 'success', detail: 'Point found', output: JSON.stringify(payload) });
            results.push({ action, status: 'success', ...payload });
          }
        } else if (action === 'moondream_detect') {
          const objectLabel = String(args.object || '').trim();
          if (!objectLabel) throw new Error('moondream_detect requires object');
          const screenshot = await ensureScreenshot(computer, lastScreenshot);
          lastScreenshot = screenshot;
          const detections = await moondream.detect(screenshot, objectLabel);
          const payload = detectPayload(detections, computer.getScreenSize());
          logs.push({ step: iteration, action, status: 'success', detail: 'Detection results', output: JSON.stringify(payload) });
          results.push({ action, status: 'success', ...payload });
        } else if (action === 'page_info') {
          const payload = await pageInfo(computer);
          const urlValue = typeof payload.url === 'string' ? payload.url : undefined;
          const stateChanged = Boolean(urlValue && urlValue !== lastPageUrl);
          if (urlValue) {
            lastPageUrl = urlValue;
          }
          payload.state_changed = stateChanged;
          const statusValue = payload.error ? 'failed' : 'success';
          logs.push({ step: iteration, action, status: statusValue, detail: 'Page info', output: JSON.stringify(payload) });
          results.push({ action, status: statusValue, ...payload });
        } else if (action === 'done') {
          doneFlag = true;
          finalResponse = String(args.final_response || '');
          break;
        } else if (action === 'fail') {
          error = String(args.error || 'unknown error');
          logs.push({ step: iteration, action, status: 'failed', detail: error });
          results.push({ action, status: 'failed', detail: error });
          doneFlag = true;
          break;
        } else {
          throw new Error(`Unknown action: ${action}`);
        }
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      logs.push({ step: iteration, action: 'batch', status: 'failed', detail: message });
      error = message;
      results.push({ action: 'batch', status: 'failed', detail: message });
    }

    appendResult(messages, 'batch', { results });

    if (
      doneFlag &&
      (!finalResponse ||
        finalResponse.includes('{{') ||
        finalResponse.includes('}}') ||
        finalResponse.toLowerCase().includes('placeholder'))
    ) {
      messages.push({
        role: 'user',
        content:
          'final_response must be non-empty and use concrete values (no placeholders). Return a corrected JSON object.',
      });
      doneFlag = false;
      finalResponse = '';
    }

    if (doneFlag) {
      const trimmed = finalResponse.trim();
      if (trimmed.startsWith('{')) {
        try {
          const repaired = jsonrepair(trimmed);
          const parsed = JSON.parse(repaired);
          if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
            throw new Error('final_response JSON is not an object');
          }
        } catch {
          messages.push({
            role: 'user',
            content: 'final_response looks like JSON but is invalid. Return a valid JSON object string.',
          });
          doneFlag = false;
          finalResponse = '';
        }
      }
    }

    if (doneFlag) {
      const urls = extractUrls(finalResponse);
      if (urls.length > 0 && !lastPageUrl) {
        messages.push({
          role: 'user',
          content:
            'You returned a URL but did not call page_info. Call page_info on the current page before final_response.',
        });
        doneFlag = false;
        finalResponse = '';
      } else if (urls.length > 0 && lastPageUrl && urls.some(url => url !== lastPageUrl)) {
        messages.push({
          role: 'user',
          content:
            'The returned URL does not match the current page_info URL. Navigate to the correct page and then return that URL.',
        });
        doneFlag = false;
        finalResponse = '';
      }
    }

    if (doneFlag) {
      const summary = `Completed ${logs.filter(log => log.status === 'success').length}/${logs.length} steps`;
      const resultPayload = {
        summary,
        final_response: finalResponse,
        steps: logs,
        answers,
      };
      return { finalResponse: JSON.stringify(resultPayload, null, 2), error };
    }
  }

  const summary = `Completed ${logs.filter(log => log.status === 'success').length}/${logs.length} steps`;
  const resultPayload = {
    summary,
    final_response: '',
    steps: logs,
    answers,
  };

  return { finalResponse: JSON.stringify(resultPayload, null, 2), error };
}

async function groqCompletion(
  groq: Groq,
  messages: Array<{ role: 'system' | 'user' | 'assistant'; content: string }>,
  options: LlmOptions,
): Promise<string> {
  const completion = await groq.chat.completions.create({
    model: MODEL_NAME,
    messages,
    temperature: options.temperature ?? 1.0,
    max_completion_tokens: options.maxCompletionTokens ?? 65536,
    top_p: options.topP ?? 1,
    reasoning_effort: options.reasoningEffort ?? 'medium',
    stream: false,
    response_format: { type: 'json_object' },
  } as any);
  return completion.choices?.[0]?.message?.content ?? '';
}

function parseJsonAction(raw: string): Record<string, unknown> {
  const start = raw.indexOf('{');
  const end = raw.lastIndexOf('}');
  if (start === -1 || end === -1 || end <= start) {
    throw new Error('No JSON object found in LLM response');
  }
  const snippet = raw.slice(start, end + 1);
  const repaired = jsonrepair(snippet);
  const parsed = JSON.parse(repaired);
  if (!parsed || typeof parsed !== 'object') {
    throw new Error('LLM JSON did not produce an object');
  }
  return parsed as Record<string, unknown>;
}

function extractUrls(finalResponse: string): string[] {
  const text = finalResponse.trim();
  if (!text.startsWith('{') || !text.endsWith('}')) return [];
  try {
    const repaired = jsonrepair(text);
    const parsed = JSON.parse(repaired);
    if (!parsed || typeof parsed !== 'object') return [];
    const urls: string[] = [];
    for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
      if (key.toLowerCase().includes('url') && typeof value === 'string') {
        urls.push(value);
      }
    }
    return urls;
  } catch {
    return [];
  }
}

function normalizeActions(payload: Record<string, unknown>): LlmAction[] {
  if (Array.isArray(payload.actions)) {
    return payload.actions
      .filter(item => item && typeof item === 'object')
      .map(item => item as LlmAction)
      .filter(item => typeof item.action === 'string' && item.action.trim().length > 0);
  }
  if (payload.action && typeof payload.action === 'string') {
    return [{ action: String(payload.action), args: (payload.args as Record<string, unknown>) || {} }];
  }
  return [];
}

function appendResult(
  messages: Array<{ role: 'system' | 'user' | 'assistant'; content: string }>,
  action: string,
  payload: unknown,
): void {
  messages.push({
    role: 'user',
    content: JSON.stringify({ type: 'action_result', action, output: payload }),
  });
}

function updateScreenshot(current?: string, fallback?: string): string | undefined {
  return current ?? fallback;
}

function updateScreenshotWithState(
  current?: string,
  previous?: string,
): { screenshot?: string; stateChanged: boolean } {
  const screenshot = updateScreenshot(current, previous);
  if (!screenshot) {
    return { screenshot: previous, stateChanged: false };
  }
  if (!previous) {
    return { screenshot, stateChanged: true };
  }
  return { screenshot, stateChanged: screenshot !== previous };
}

function status(error?: string): 'success' | 'failed' {
  return error ? 'failed' : 'success';
}

function coerceCoords(args: Record<string, unknown>, screenSize: ScreenSize): [number, number] {
  if (args.x === undefined || args.y === undefined) {
    throw new Error('x and y are required');
  }
  if (typeof args.x === 'string' && (args.x.includes('{') || args.x.includes('}'))) {
    throw new Error('x must be a number, not a placeholder');
  }
  if (typeof args.y === 'string' && (args.y.includes('{') || args.y.includes('}'))) {
    throw new Error('y must be a number, not a placeholder');
  }
  const x = Number(args.x);
  const y = Number(args.y);
  if (x >= 0 && x <= 1 && y >= 0 && y <= 1) {
    return [Math.round(x * COORDINATE_SCALE), Math.round(y * COORDINATE_SCALE)];
  }
  if (x >= 0 && x <= COORDINATE_SCALE && y >= 0 && y <= COORDINATE_SCALE) {
    return [Math.round(x), Math.round(y)];
  }
  const width = screenSize?.width ?? DEFAULT_SCREEN_SIZE.width;
  const height = screenSize?.height ?? DEFAULT_SCREEN_SIZE.height;
  return [
    Math.round((x / width) * COORDINATE_SCALE),
    Math.round((y / height) * COORDINATE_SCALE),
  ];
}

async function ensureScreenshot(computer: ComputerTool, lastScreenshot?: string): Promise<string> {
  if (lastScreenshot) return lastScreenshot;
  const result = await computer.screenshot();
  if (result.error || !result.base64Image) {
    throw new Error(result.error || 'Failed to capture screenshot');
  }
  return result.base64Image;
}

function pointPayload(point: MoondreamPoint, screenSize: ScreenSize): Record<string, unknown> {
  const xNorm = Math.round(point.x * COORDINATE_SCALE);
  const yNorm = Math.round(point.y * COORDINATE_SCALE);
  const xPx = Math.round(point.x * screenSize.width);
  const yPx = Math.round(point.y * screenSize.height);
  return {
    x: point.x,
    y: point.y,
    x_norm: xNorm,
    y_norm: yNorm,
    x_px: xPx,
    y_px: yPx,
    screen: { width: screenSize.width, height: screenSize.height },
  };
}

function detectPayload(
  detections: Array<{ x_min: number; y_min: number; x_max: number; y_max: number }>,
  screenSize: ScreenSize,
): Record<string, unknown> {
  const objects = detections.map(det => ({
    x_min: det.x_min,
    y_min: det.y_min,
    x_max: det.x_max,
    y_max: det.y_max,
    x_min_norm: Math.round(det.x_min * COORDINATE_SCALE),
    y_min_norm: Math.round(det.y_min * COORDINATE_SCALE),
    x_max_norm: Math.round(det.x_max * COORDINATE_SCALE),
    y_max_norm: Math.round(det.y_max * COORDINATE_SCALE),
    x_min_px: Math.round(det.x_min * screenSize.width),
    y_min_px: Math.round(det.y_min * screenSize.height),
    x_max_px: Math.round(det.x_max * screenSize.width),
    y_max_px: Math.round(det.y_max * screenSize.height),
  }));
  return { objects, screen: { width: screenSize.width, height: screenSize.height } };
}

async function pageInfo(computer: ComputerTool): Promise<Record<string, unknown>> {
  try {
    const { createRequire } = await import('node:module');
    const require = createRequire(import.meta.url);
    let playwright: any;
    try {
      playwright = require('playwright-core');
    } catch {
      return { error: 'playwright-core not installed' };
    }
    const browserInfo = await computer.getKernel().browsers.retrieve(computer.getSessionId());
    const cdpUrl = browserInfo?.cdp_ws_url as string | undefined;
    if (!cdpUrl) return { error: 'cdp url not available' };

    const browser = await playwright.chromium.connectOverCDP(cdpUrl);
    const pages: any[] = [];
    for (const context of browser.contexts()) {
      pages.push(...context.pages());
    }
    const page = pages.length > 0 ? pages[pages.length - 1] : await browser.newPage();
    const title = await page.title();
    const url = page.url();
    await browser.close();
    return { url, title };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}

async function postWait(
  action: string,
  args: Record<string, unknown>,
  options: LlmOptions,
): Promise<void> {
  const waitActions = new Set([
    'navigate',
    'click_at',
    'hover_at',
    'type_text_at',
    'scroll_document',
    'scroll_at',
    'go_back',
    'go_forward',
    'key_combination',
    'drag_and_drop',
  ]);
  if (!waitActions.has(action)) return;
  const override = args.post_wait_ms;
  const defaultWait = options.postActionWaitMs ?? 500;
  const waitMs = typeof override === 'number' ? override : defaultWait;
  if (waitMs > 0) {
    await sleep(waitMs);
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}
