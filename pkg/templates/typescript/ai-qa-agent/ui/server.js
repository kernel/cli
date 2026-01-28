import express from 'express';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import Anthropic from '@anthropic-ai/sdk';
import OpenAI from 'openai';
import { GoogleGenerativeAI } from '@google/generative-ai';
import { config } from 'dotenv';

// Load environment variables
config();

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware
app.use(express.json());
app.use(express.static(__dirname));

// Import the QA agent logic (we'll need to export the functions from index.ts)
// For now, we'll duplicate the necessary code here

// Vision Provider implementations
class ClaudeVisionProvider {
  constructor(apiKey) {
    this.name = "Claude (Anthropic)";
    this.client = new Anthropic({ apiKey });
  }

  async analyzeScreenshot(screenshot, prompt) {
    const base64Image = screenshot.toString("base64");
    const response = await this.client.messages.create({
      model: "claude-3-5-sonnet-20241022",
      max_tokens: 2048,
      messages: [{
        role: "user",
        content: [
          { type: "image", source: { type: "base64", media_type: "image/png", data: base64Image } },
          { type: "text", text: prompt },
        ],
      }],
    });
    const textContent = response.content.find((block) => block.type === "text");
    return textContent && textContent.type === "text" ? textContent.text : "";
  }
}

class GPT4oVisionProvider {
  constructor(apiKey) {
    this.name = "GPT-4o (OpenAI)";
    this.client = new OpenAI({ apiKey });
  }

  async analyzeScreenshot(screenshot, prompt) {
    const base64Image = screenshot.toString("base64");
    const response = await this.client.chat.completions.create({
      model: "gpt-4o",
      max_tokens: 2048,
      messages: [{
        role: "user",
        content: [
          { type: "image_url", image_url: { url: `data:image/png;base64,${base64Image}` } },
          { type: "text", text: prompt },
        ],
      }],
    });
    return response.choices[0]?.message?.content || "";
  }
}

class GeminiVisionProvider {
  constructor(apiKey) {
    this.name = "Gemini (Google)";
    this.client = new GoogleGenerativeAI(apiKey);
  }

  async analyzeScreenshot(screenshot, prompt) {
    const model = this.client.getGenerativeModel({ model: "gemini-2.0-flash-exp" });
    const result = await model.generateContent([
      { inlineData: { mimeType: "image/png", data: screenshot.toString("base64") } },
      prompt,
    ]);
    return result.response.text();
  }
}

function createVisionProvider(model) {
  switch (model) {
    case "claude":
      if (!process.env.ANTHROPIC_API_KEY) throw new Error("ANTHROPIC_API_KEY is required");
      return new ClaudeVisionProvider(process.env.ANTHROPIC_API_KEY);
    case "gpt4o":
      if (!process.env.OPENAI_API_KEY) throw new Error("OPENAI_API_KEY is required");
      return new GPT4oVisionProvider(process.env.OPENAI_API_KEY);
    case "gemini":
      if (!process.env.GOOGLE_API_KEY) throw new Error("GOOGLE_API_KEY is required");
      return new GeminiVisionProvider(process.env.GOOGLE_API_KEY);
    default:
      throw new Error(`Unknown model: ${model}`);
  }
}

// Store active SSE connections
const sseConnections = new Map();

// SSE endpoint for progress updates
app.get('/api/progress/:sessionId', (req, res) => {
  const sessionId = req.params.sessionId;
  
  res.setHeader('Content-Type', 'text/event-stream');
  res.setHeader('Cache-Control', 'no-cache');
  res.setHeader('Connection', 'keep-alive');
  
  // Store this connection
  sseConnections.set(sessionId, res);
  
  // Send initial connection message
  res.write(`data: ${JSON.stringify({ type: 'connected' })}\n\n`);
  
  // Clean up on close
  req.on('close', () => {
    sseConnections.delete(sessionId);
  });
});

// Helper to send progress updates
function sendProgress(sessionId, data) {
  const connection = sseConnections.get(sessionId);
  if (connection) {
    connection.write(`data: ${JSON.stringify(data)}\n\n`);
  }
}

// API endpoint to run QA analysis
app.post('/api/run-qa', async (req, res) => {
  const sessionId = `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  
  try {
    const input = req.body;
    
    console.log('Received request:', JSON.stringify(input, null, 2));
    
    // Validate input
    if (!input.url) {
      return res.status(400).json({ error: 'URL is required' });
    }
    
    // Send session ID immediately
    res.json({ sessionId });
    
    // Import and run the QA task from index.ts
    const { default: runQA } = await import('../index.ts');
    
    // Send progress: Starting
    sendProgress(sessionId, {
      type: 'status',
      step: 'starting',
      message: `Starting analysis of ${input.url}...`
    });
    
    // Create a wrapper that sends progress updates
    const progressCallback = (step, message) => {
      sendProgress(sessionId, { type: 'status', step, message });
    };
    
    // Run the QA analysis with progress callback
    const result = await runQA(undefined, input, progressCallback);
    
    // Send final result
    sendProgress(sessionId, {
      type: 'complete',
      result
    });
    
  } catch (error) {
    console.error('Error running QA:', error);
    sendProgress(sessionId, {
      type: 'error',
      error: error.message || 'Failed to run QA analysis'
    });
  } finally {
    // Close SSE connection after a delay
    setTimeout(() => {
      const connection = sseConnections.get(sessionId);
      if (connection) {
        connection.end();
        sseConnections.delete(sessionId);
      }
    }, 1000);
  }
});

// Health check endpoint
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', message: 'QA Agent UI Server is running' });
});

// Start server
app.listen(PORT, () => {
  console.log(`\n🚀 QA Agent UI Server running at http://localhost:${PORT}`);
  console.log(`📊 Open http://localhost:${PORT} in your browser to use the QA Agent\n`);
});
