export const clientProtocols = ['codex', 'openai', 'claude', 'gemini'] as const
export type ClientProtocol = (typeof clientProtocols)[number]

// Model IDs are user input. Quote the complete JSON/URL argument before copying a shell command.
const shellLiteral = (value: string) => `'${value.replaceAll("'", "'\"'\"'")}'`

export function clientConfiguration(
  protocol: ClientProtocol,
  origin: string,
  model: string,
) {
  const baseURL =
    protocol === 'codex' || protocol === 'openai' ? `${origin}/v1` : origin
  const selectedModel = model.trim()
  if (protocol === 'codex') {
    return {
      baseURL,
      configuration: `${selectedModel ? `model = ${JSON.stringify(selectedModel)}\n` : ''}model_provider = "sublane"\n\n[model_providers.sublane]\nname = "SubLane"\nbase_url = ${JSON.stringify(baseURL)}\nenv_key = "SUBLANE_API_KEY"\nwire_api = "responses"\nrequires_openai_auth = false\nsupports_websockets = true`,
    }
  }
  const id = selectedModel || 'YOUR_MODEL_ID'
  const url =
    protocol === 'claude'
      ? `${origin}/v1/messages`
      : protocol === 'openai'
        ? `${baseURL}/chat/completions`
        : `${origin}/v1beta/models/${encodeURIComponent(id)}:generateContent`
  const body =
    protocol !== 'gemini'
      ? {
          model: id,
          ...(protocol === 'claude' ? { max_tokens: 1024 } : {}),
          messages: [{ role: 'user', content: 'Hello' }],
        }
      : { contents: [{ role: 'user', parts: [{ text: 'Hello' }] }] }
  const authorization = {
    claude: 'x-api-key: $SUBLANE_API_KEY',
    gemini: 'x-goog-api-key: $SUBLANE_API_KEY',
    openai: 'Authorization: Bearer $SUBLANE_API_KEY',
  }[protocol]
  return {
    baseURL,
    configuration: [
      `curl ${shellLiteral(url)}`,
      `  -H "${authorization}"`,
      ...(protocol === 'claude'
        ? ["  -H 'anthropic-version: 2023-06-01'"]
        : []),
      "  -H 'content-type: application/json'",
      `  --data ${shellLiteral(JSON.stringify(body))}`,
    ].join(' \\\n'),
  }
}
