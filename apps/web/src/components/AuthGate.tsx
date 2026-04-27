import { useEffect, useState, type ReactNode } from 'react';
import { Eye, LogIn } from 'lucide-react';
import { getAuthStatus, login } from '../api/sunny';
import './AuthGate.css';

interface Props {
  children: ReactNode;
}

type Phase = 'checking' | 'login' | 'ok';

export default function AuthGate({ children }: Props) {
  const [phase, setPhase] = useState<Phase>('checking');
  const [error, setError] = useState<string | null>(null);
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const recheck = async () => {
    try {
      const s = await getAuthStatus();
      if (!s.enabled || s.loggedIn) setPhase('ok');
      else setPhase('login');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setPhase('login');
    }
  };

  useEffect(() => {
    recheck();
  }, []);

  if (phase === 'checking') {
    return <div className="auth-loading">Loading…</div>;
  }
  if (phase === 'ok') return <>{children}</>;

  return (
    <div className="auth-gate">
      <form
        className="auth-card"
        onSubmit={async (e) => {
          e.preventDefault();
          setSubmitting(true);
          setError(null);
          try {
            await login(password);
            await recheck();
          } catch (err) {
            setError(err instanceof Error ? err.message : String(err));
          } finally {
            setSubmitting(false);
          }
        }}
      >
        <div className="auth-brand">
          <Eye size={32} />
          <div>
            <h1>Sunny</h1>
            <p>Physical Observability</p>
          </div>
        </div>
        <label>
          Password
          <input
            type="password"
            value={password}
            autoFocus
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Enter password"
          />
        </label>
        {error && <p className="auth-error">{error}</p>}
        <button type="submit" disabled={submitting || !password}>
          <LogIn size={14} /> {submitting ? 'Signing in…' : 'Sign in'}
        </button>
        <p className="auth-help">
          Set with <code>SUNNY_PASSWORD_HASH</code>. Generate a hash with
          <code>sunny-cli hash-password &lt;your-password&gt;</code>.
        </p>
      </form>
    </div>
  );
}
