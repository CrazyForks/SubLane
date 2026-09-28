import { spawnSync } from 'node:child_process'
import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { clientConfiguration } from './client'

it.each(['claude', 'gemini', 'openai'] as const)(
  'copies a safe %s request without executing model text',
  (protocol) => {
    const directory = mkdtempSync(join(tmpdir(), 'sublane-client-'))
    try {
      writeFileSync(
        join(directory, 'curl'),
        '#!/bin/sh\nprintf "%s\\000" "$@" > "$CLIENT_ARGS"\n',
        { mode: 0o755 },
      )
      const model = "synthetic'; touch injected; echo '$(touch substituted)"
      const { configuration } = clientConfiguration(
        protocol,
        'https://gateway.example.test',
        model,
      )
      const result = spawnSync('/bin/sh', ['-c', configuration], {
        cwd: directory,
        env: {
          ...process.env,
          PATH: `${directory}:${process.env.PATH}`,
          SUBLANE_API_KEY: 'synthetic-key',
          CLIENT_ARGS: join(directory, 'args'),
        },
        encoding: 'utf8',
      })
      expect(result.status, result.stderr).toBe(0)
      expect(existsSync(join(directory, 'injected'))).toBe(false)
      expect(existsSync(join(directory, 'substituted'))).toBe(false)
      const args = readFileSync(join(directory, 'args'), 'utf8').split('\0')
      const body = JSON.parse(args[args.indexOf('--data') + 1])
      if (protocol === 'claude') {
        expect(body.model).toBe(model)
        expect(args).toContain('https://gateway.example.test/v1/messages')
        expect(args).toContain('x-api-key: synthetic-key')
      } else if (protocol === 'gemini') {
        expect(args).toContain(
          `https://gateway.example.test/v1beta/models/${encodeURIComponent(model)}:generateContent`,
        )
        expect(body.contents[0].parts[0].text).toBe('Hello')
        expect(args).toContain('x-goog-api-key: synthetic-key')
      } else {
        expect(args).toContain(
          'https://gateway.example.test/v1/chat/completions',
        )
        expect(args).toContain('Authorization: Bearer synthetic-key')
        expect(body).toEqual({
          model,
          messages: [{ role: 'user', content: 'Hello' }],
        })
      }
    } finally {
      rmSync(directory, { recursive: true, force: true })
    }
  },
)

it('keeps Codex configuration and distinguishes native base URLs', () => {
  const codex = clientConfiguration(
    'codex',
    'https://gateway.example.test',
    'synthetic-model',
  )
  expect(codex.baseURL).toBe('https://gateway.example.test/v1')
  expect(codex.configuration).toContain('model = "synthetic-model"')
  expect(codex.configuration).toContain('wire_api = "responses"')
  expect(
    clientConfiguration('claude', 'https://gateway.example.test', '').baseURL,
  ).toBe('https://gateway.example.test')
  expect(
    clientConfiguration('gemini', 'https://gateway.example.test', '')
      .configuration,
  ).toContain('YOUR_MODEL_ID')
})

it('uses the OpenAI base URL and a model placeholder for Cline setup', () => {
  const result = clientConfiguration(
    'openai',
    'https://gateway.example.test',
    '  ',
  )
  expect(result.baseURL).toBe('https://gateway.example.test/v1')
  expect(result.configuration).toContain('YOUR_MODEL_ID')
})
