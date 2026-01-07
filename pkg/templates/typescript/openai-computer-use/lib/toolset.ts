const shared = [
  {
    type: 'function',
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
  },
  {
    type: 'function',
    name: 'back',
    description: 'Navigate back in the browser history.',
    parameters: {
      type: 'object',
      properties: {},
      additionalProperties: false,
    },
  },
  {
    type: 'function',
    name: 'forward',
    description: 'Navigate forward in the browser history.',
    parameters: {
      type: 'object',
      properties: {},
      additionalProperties: false,
    },
  },
];

export default { shared };
