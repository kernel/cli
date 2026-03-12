export const batchInstructions = `You have three ways to perform actions:
1. The standard computer tool — use for single actions when you need screenshot feedback after each step.
2. batch_computer_actions — use to execute multiple actions at once when you can predict the outcome.
3. computer_use_extra — use high-level browser actions: goto, back, and url.

ALWAYS prefer batch_computer_actions when performing predictable sequences like:
- Clicking a text field, typing text, and pressing Enter
- Any sequence where you don't need to see intermediate results

Use computer_use_extra for:
- action="goto" only when changing the page URL
- action="back" to go back in history
- action="url" to read the exact current URL

When interacting with page content (search boxes, forms, chat inputs):
- Click the target input first, then type.
- Do not use URL-navigation actions for in-page text entry.

For drag actions in batch_computer_actions:
- Always include a path field.
- path must be an array of at least two points.
- Each point must be an object like {"x": 123, "y": 456}.`;

export const batchComputerTool = {
  type: 'function' as const,
  name: 'batch_computer_actions',
  description:
    'Execute multiple computer actions in sequence without waiting for ' +
    'screenshots between them. Use this when you can predict the outcome of a ' +
    'sequence of actions without needing intermediate visual feedback. After all ' +
    'actions execute, a single screenshot is taken and returned.\n\n' +
    'PREFER this over individual computer actions when:\n' +
    '- Typing text followed by pressing Enter\n' +
    '- Clicking a field and then typing into it\n' +
    "- Any sequence where intermediate screenshots aren't needed\n\n" +
    'Constraint: return-value actions (url, screenshot) can appear at most once ' +
    'and only as the final action in the batch.',
  parameters: {
    type: 'object',
    properties: {
      actions: {
        type: 'array',
        description: 'Ordered list of actions to execute',
        items: {
          type: 'object',
          properties: {
            type: {
              type: 'string',
              enum: [
                'click',
                'double_click',
                'type',
                'keypress',
                'scroll',
                'move',
                'drag',
                'wait',
                'goto',
                'back',
                'url',
                'screenshot',
              ],
            },
            x: { type: 'number' },
            y: { type: 'number' },
            text: { type: 'string' },
            url: { type: 'string' },
            keys: { type: 'array', items: { type: 'string' } },
            hold_keys: { type: 'array', items: { type: 'string' } },
            button: { type: 'string' },
            scroll_x: { type: 'number' },
            scroll_y: { type: 'number' },
            path: {
              type: 'array',
              description: 'Required for drag actions. Provide at least two points as objects with x/y coordinates.',
              items: {
                type: 'object',
                properties: {
                  x: { type: 'number' },
                  y: { type: 'number' },
                },
                required: ['x', 'y'],
              },
            },
          },
          required: ['type'],
        },
      },
    },
    required: ['actions'],
  },
  strict: false,
};

export const computerUseExtraTool = {
  type: 'function' as const,
  name: 'computer_use_extra',
  description: 'High-level browser actions for navigation and URL retrieval.',
  parameters: {
    type: 'object',
    properties: {
      action: {
        type: 'string',
        enum: ['goto', 'back', 'url'],
        description: 'Action to perform: goto, back, or url.',
      },
      url: {
        type: 'string',
        description: 'Required when action is goto. Fully qualified URL to navigate to.',
      },
    },
    required: ['action'],
  },
  strict: false,
};
