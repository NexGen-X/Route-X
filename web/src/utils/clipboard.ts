export async function copyTextToClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fallback ke textarea jika navigator.clipboard diblokir di headless / HTTP lokal
    }
  }

  // Fallback universal untuk LAN non-HTTPS, WebView, atau browser tanpa izin clipboard
  const textArea = document.createElement('textarea');
  textArea.value = text;
  textArea.style.position = 'fixed';
  textArea.style.left = '-999999px';
  textArea.style.top = '-999999px';
  textArea.setAttribute('readonly', '');
  document.body.appendChild(textArea);
  textArea.focus();
  textArea.select();

  try {
    const successful = document.execCommand('copy');
    if (!successful) {
      throw new Error('Clipboard tidak dapat diakses di peramban ini.');
    }
  } finally {
    document.body.removeChild(textArea);
  }
}
