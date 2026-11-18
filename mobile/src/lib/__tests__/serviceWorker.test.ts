import { canUseServiceWorker } from '../serviceWorker';

const at = (url: string) => {
  const u = new URL(url);
  return { protocol: u.protocol, hostname: u.hostname, port: u.port };
};

describe('canUseServiceWorker', () => {
  it('allows HTTPS origins', () => {
    expect(canUseServiceWorker(at('https://pi.tail1234.ts.net'))).toBe(true);
    expect(canUseServiceWorker(at('https://yume.example.com:8443'))).toBe(true);
  });
  it('allows the secure local origins browsers accept', () => {
    expect(canUseServiceWorker(at('http://localhost:7654'))).toBe(true);
    expect(canUseServiceWorker(at('http://127.0.0.1:7654'))).toBe(true);
    expect(canUseServiceWorker(at('http://[::1]:7654'))).toBe(true);
    expect(canUseServiceWorker(at('http://yume.localhost:7654'))).toBe(true);
  });
  it('never registers on plain http to another machine', () => {
    expect(canUseServiceWorker(at('http://100.101.102.103:7654'))).toBe(false);
    expect(canUseServiceWorker(at('http://192.168.1.20:7654'))).toBe(false);
    expect(canUseServiceWorker(at('http://raspberrypi.tail1234.ts.net:7654'))).toBe(false);
  });
  it('skips the Metro dev server and file pages', () => {
    expect(canUseServiceWorker(at('http://localhost:8081'))).toBe(false);
    expect(canUseServiceWorker(at('file:///tmp/index.html'))).toBe(false);
  });
});
