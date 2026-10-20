/// <reference types="svelte" />
/// <reference types="vite/client" />

// Version « legacy » de pdf.js : fonctionne sur les WebKitGTK plus anciens. Les
// types sont ceux du paquet principal ; le worker est chargé comme ressource.
declare module 'pdfjs-dist/legacy/build/pdf.min.mjs' {
  export * from 'pdfjs-dist';
}
declare module 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url' {
  const url: string;
  export default url;
}
