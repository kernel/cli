export interface MoondreamPoint {
  x: number;
  y: number;
}

export class MoondreamError extends Error {}

export class MoondreamClient {
  private apiKey: string;
  private baseUrl: string;
  private timeoutMs: number;

  constructor({
    apiKey,
    baseUrl = 'https://api.moondream.ai/v1',
    timeoutMs = 30000,
  }: {
    apiKey: string;
    baseUrl?: string;
    timeoutMs?: number;
  }) {
    this.apiKey = apiKey;
    this.baseUrl = baseUrl.replace(/\/$/, '');
    this.timeoutMs = timeoutMs;
  }

  async query(imageBase64: string, question: string, reasoning?: boolean): Promise<string> {
    const payload: Record<string, unknown> = {
      image_url: toDataUrl(imageBase64),
      question,
    };
    if (reasoning !== undefined) payload.reasoning = reasoning;
    const data = await this.post('/query', payload);
    if (typeof data.answer !== 'string') {
      throw new MoondreamError('Moondream query returned an invalid response');
    }
    return data.answer;
  }

  async caption(imageBase64: string, length: 'short' | 'normal' | 'long' = 'normal'): Promise<string> {
    const payload = {
      image_url: toDataUrl(imageBase64),
      length,
      stream: false,
    };
    const data = await this.post('/caption', payload);
    if (typeof data.caption !== 'string') {
      throw new MoondreamError('Moondream caption returned an invalid response');
    }
    return data.caption;
  }

  async point(imageBase64: string, objectLabel: string): Promise<MoondreamPoint | null> {
    const payload = {
      image_url: toDataUrl(imageBase64),
      object: objectLabel,
    };
    const data = await this.post('/point', payload);
    if (!Array.isArray(data.points) || data.points.length === 0) {
      return null;
    }
    const point = data.points[0] as { x?: number; y?: number };
    if (typeof point?.x !== 'number' || typeof point?.y !== 'number') {
      return null;
    }
    return { x: point.x, y: point.y };
  }

  async detect(
    imageBase64: string,
    objectLabel: string,
  ): Promise<Array<{ x_min: number; y_min: number; x_max: number; y_max: number }>> {
    const payload = {
      image_url: toDataUrl(imageBase64),
      object: objectLabel,
    };
    const data = await this.post('/detect', payload);
    if (!Array.isArray(data.objects)) {
      return [];
    }
    const results: Array<{ x_min: number; y_min: number; x_max: number; y_max: number }> = [];
    for (const item of data.objects) {
      const box = item as { x_min?: number; y_min?: number; x_max?: number; y_max?: number };
      if ([box.x_min, box.y_min, box.x_max, box.y_max].every(v => typeof v === 'number')) {
        results.push({
          x_min: box.x_min as number,
          y_min: box.y_min as number,
          x_max: box.x_max as number,
          y_max: box.y_max as number,
        });
      }
    }
    return results;
  }

  private async post(path: string, payload: Record<string, unknown>): Promise<Record<string, any>> {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const response = await fetch(`${this.baseUrl}${path}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Moondream-Auth': this.apiKey,
        },
        body: JSON.stringify(payload),
        signal: controller.signal,
      });

      if (!response.ok) {
        const text = await response.text();
        throw new MoondreamError(`Moondream API error ${response.status}: ${text}`);
      }

      const data = await response.json();
      if (!data || typeof data !== 'object') {
        throw new MoondreamError('Moondream API returned unexpected response type');
      }
      return data as Record<string, any>;
    } catch (error) {
      if (error instanceof MoondreamError) throw error;
      if (error instanceof Error) {
        throw new MoondreamError(error.message);
      }
      throw new MoondreamError(String(error));
    } finally {
      clearTimeout(timeout);
    }
  }
}

function toDataUrl(imageBase64: string): string {
  return `data:image/png;base64,${imageBase64}`;
}
