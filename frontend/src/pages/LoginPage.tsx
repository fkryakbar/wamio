import { useState, useCallback } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { useAuthStore } from '../stores/authStore';

interface LoginPageProps {
  mode: 'initial' | 'add';
  onBeginPairing: (label: string) => Promise<void>;
  onCancel?: () => void;
}

export function LoginPage({ mode, onBeginPairing, onCancel }: LoginPageProps) {
  const {
    connectionState,
    qrCode,
    accountId,
    errorMessage,
    setAccountId,
    setError,
  } = useAuthStore();

  const [isConnecting, setIsConnecting] = useState(false);

  const handleConnect = useCallback(async () => {
    if (!accountId.trim()) {
      setError('Masukkan nama akun terlebih dahulu');
      return;
    }
    setIsConnecting(true);
    setError(null);
    try {
      await onBeginPairing(accountId.trim());
    } catch (err: any) {
      setError(err?.message || 'Gagal memulai penambahan akun');
      setIsConnecting(false);
    }
  }, [accountId, onBeginPairing, setError]);

  const handleRetry = useCallback(() => {
    setError(null);
    handleConnect();
  }, [handleConnect, setError]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && connectionState !== 'qr_ready' && connectionState !== 'connected') {
      handleConnect();
    }
  }, [handleConnect, connectionState]);

  const renderStatus = () => {
    if (errorMessage) {
      return (
        <div className="login-card__status login-card__status--error">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/>
          </svg>
          {errorMessage}
        </div>
      );
    }
    if (isConnecting || connectionState === 'connecting') {
      return (
        <div className="login-card__status login-card__status--connecting">
          <div className="spinner spinner--sm" />
          Menghubungkan...
        </div>
      );
    }
    if (connectionState === 'qr_ready') {
      return (
        <div className="login-card__status login-card__status--connecting">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
            <path d="M17 1H7c-1.1 0-2 .9-2 2v18c0 1.1.9 2 2 2h10c1.1 0 2-.9 2-2V3c0-1.1-.9-2-2-2zm0 18H7V5h10v14z"/>
          </svg>
          Scan QR Code dengan HP Anda
        </div>
      );
    }
    if (connectionState === 'connected') {
      return (
        <div className="login-card__status login-card__status--success">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
            <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/>
          </svg>
          Terhubung!
        </div>
      );
    }
    return null;
  };

  return (
    <div className="login-page">
      <div className="login-card" onKeyDown={handleKeyDown}>
        {/* Logo */}
        <div className="login-card__logo">
          <svg viewBox="0 0 39 39" xmlns="http://www.w3.org/2000/svg">
            <path d="M10.7 32.8l.6.3c2.5 1.5 5.3 2.2 8.1 2.2 8.8 0 16-7.2 16-16 0-4.2-1.7-8.3-4.7-11.3s-7-4.7-11.3-4.7c-8.8 0-16 7.2-15.9 16.1 0 3 .9 5.9 2.4 8.4l.4.6-1.5 5.5 5.9-1.1z"/>
            <path fill="var(--bg-secondary)" d="M32.4 6.4C29 2.9 24.3 1 19.5 1 9.3 1 1.1 9.3 1.2 19.4c0 3.2.9 6.3 2.4 9.1L1 38l9.7-2.5c2.7 1.5 5.7 2.2 8.7 2.2 10.1 0 18.3-8.3 18.3-18.4 0-4.9-1.9-9.5-5.3-12.9zM19.5 34.6c-2.7 0-5.4-.7-7.7-2.1l-.6-.3-5.8 1.5L6.9 28l-.4-.6c-4.4-7.1-2.3-16.5 4.9-20.9s16.5-2.3 20.9 4.9 2.3 16.5-4.9 20.9c-2.3 1.5-5.1 2.3-7.9 2.3zm8.8-11.1l-1.1-.5s-1.6-.7-2.6-1.2c-.1 0-.2-.1-.3-.1-.3 0-.5.1-.7.3 0 0-.1.1-1.5 1.7-.1.2-.3.3-.5.3h-.1c-.1 0-.3-.1-.4-.2l-.5-.2c-1.1-.5-2.1-1.1-2.9-1.9-.2-.2-.5-.4-.7-.6-.7-.7-1.4-1.5-1.9-2.4l-.1-.2c-.1-.1-.1-.2-.2-.4 0-.2 0-.4.1-.5 0 0 .4-.5.7-.8.2-.2.3-.5.5-.7.2-.3.3-.7.2-1-.1-.5-1.3-3.2-1.6-3.8-.2-.3-.4-.4-.7-.5h-1.1c-.2 0-.4.1-.6.1l-.1.1c-.2.1-.4.3-.6.4-.2.2-.3.4-.5.6-.7.9-1.1 2-1.1 3.1 0 .8.2 1.6.5 2.3l.1.3c.9 1.9 2.1 3.6 3.7 5.1l.4.4c.3.3.6.5.8.8 2.1 1.8 4.5 3.1 7.2 3.8.3.1.7.1 1 .2h1c.5 0 1.1-.2 1.5-.4.3-.2.5-.2.7-.4l.2-.2c.2-.2.4-.3.6-.5s.3-.4.5-.6c.2-.4.3-.9.4-1.4v-.7s-.1-.1-.3-.2z"/>
          </svg>
          <h1>Wamio</h1>
        </div>

        {/* Subtitle */}
        <p className="login-card__subtitle">
          Kirim dan terima pesan WhatsApp langsung dari desktop Anda.
          <br />
          {mode === 'add' ? 'Masukkan nama akun, lalu scan QR code untuk menambahkannya.' : 'Masukkan nama akun, lalu scan QR code untuk menghubungkannya.'}
        </p>

        {/* Account ID Input */}
        <div className="login-card__account-input">
          <label htmlFor="account-id">Nama akun</label>
          <input
            id="account-id"
            type="text"
            value={accountId}
            onChange={(e) => setAccountId(e.target.value)}
            placeholder="Contoh: Kantor"
            disabled={connectionState === 'qr_ready' || connectionState === 'connected'}
          />
        </div>

        {/* QR Code Display */}
        {qrCode && (
          <div className="login-card__qr-container">
            <QRCodeSVG
              value={qrCode}
              size={264}
              bgColor="#ffffff"
              fgColor="#111b21"
              level="M"
              includeMargin={false}
            />
            {errorMessage && (
              <div className="login-card__qr-overlay login-card__qr-overlay--expired">
                <button className="login-card__retry-btn" onClick={handleRetry}>
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/>
                  </svg>
                  Muat Ulang QR
                </button>
              </div>
            )}
          </div>
        )}

        {/* Connect Button */}
        {connectionState !== 'qr_ready' && connectionState !== 'connected' && (
          <button
            className="login-card__connect-btn"
            onClick={handleConnect}
            disabled={isConnecting || connectionState === 'connecting'}
          >
            {isConnecting || connectionState === 'connecting' ? (
              <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '8px' }}>
                <div className="spinner spinner--sm" style={{ borderTopColor: 'white', borderColor: 'rgba(255,255,255,0.3)' }} />
                Menghubungkan...
              </span>
            ) : (
              'Hubungkan WhatsApp'
            )}
          </button>
        )}

        {mode === 'add' && onCancel && (
          <button type="button" className="login-card__retry-btn" onClick={onCancel} disabled={isConnecting || connectionState === 'connecting'}>
            Kembali ke akun sebelumnya
          </button>
        )}

        {/* Status */}
        {renderStatus()}
      </div>
    </div>
  );
}
