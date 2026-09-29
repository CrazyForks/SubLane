import { spawn } from 'node:child_process'

const children = [
  spawn('make', ['dev-api'], { stdio: 'inherit', detached: process.platform !== 'win32' }),
  spawn('pnpm', ['--dir', 'web', 'dev', ...process.argv.slice(2)], { stdio: 'inherit', detached: process.platform !== 'win32' }),
]
let stopping = false
const stop = (exitCode = 0) => {
  if (stopping) return
  stopping = true
  for (const child of children) {
    if (child.pid) {
      try {
        // Air and Vite spawn descendants; signal the group to avoid orphaned servers.
        if (process.platform === 'win32') child.kill('SIGTERM')
        else process.kill(-child.pid, 'SIGTERM')
      } catch (error) { if (error.code !== 'ESRCH') console.error(error.message) }
    }
  }
  process.exitCode = exitCode
}
for (const child of children) {
  child.on('error', (error) => { console.error(error.message); stop(1) })
  child.on('exit', (code) => stop(code ?? 0))
}
process.on('SIGINT', () => stop())
process.on('SIGTERM', () => stop())
