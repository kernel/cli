export const batchInstructions = `You have two ways to perform actions:
1. The standard computer tool — use for single actions when you need screenshot feedback after each step.
2. batch_computer_actions — use to execute multiple actions at once when you can predict the outcome.

ALWAYS prefer batch_computer_actions when performing predictable sequences like:
- Clicking a text field, typing text, and pressing Enter
- Typing a URL and pressing Enter
- Any sequence where you don't need to see intermediate results`;

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
    '- Any sequence where intermediate screenshots are not needed',
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
              enum: ['click', 'double_click', 'type', 'keypress', 'scroll', 'move', 'drag', 'wait'],
            },
            x: { type: 'number' },
            y: { type: 'number' },
            text: { type: 'string' },
            keys: { type: 'array', items: { type: 'string' } },
            button: { type: 'string' },
            scroll_x: { type: 'number' },
            scroll_y: { type: 'number' },
          },
          required: ['type'],
        },
      },
    },
    required: ['actions'],
  },
  strict: false,
};

export const navigationTools = [
  {
    type: 'function' as const,
    name: 'goto',
    description: 'Go to a specific URL.',
    parameters: {
      type: 'object',
      properties: {
        url: {
          type: 'string',
          description: 'Fully qualified URL to navigate to.',
        },
      },
      additionalProperties: false,
      required: ['url'],
    },
    strict: false,
  },
  {
    type: 'function' as const,
    name: 'back',
    description: 'Navigate back in the browser history.',
    parameters: {
      type: 'object',
      properties: {},
      additionalProperties: false,
    },
    strict: false,
  },
  {
    type: 'function' as const,
    name: 'forward',
    description: 'Navigate forward in the browser history.',
    parameters: {
      type: 'object',
      properties: {},
      additionalProperties: false,
    },
    strict: false,
  },
];
