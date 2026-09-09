export async function copyTextToClipboard(text: string): Promise<void> {
  if (!navigator.clipboard?.writeText) {
    throw new Error('Clipboard tidak tersedia di browser ini.');
  }

  await navigator.clipboard.writeText(text);
}
