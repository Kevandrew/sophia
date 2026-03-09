import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import { readBootstrap } from './bootstrap';
import './index.css';

const bootstrap = readBootstrap();

function wireCloseButton() {
  const closeButton = document.getElementById('close-preview-btn');
  if (!closeButton) {
    return;
  }
  closeButton.addEventListener('click', async () => {
    const closeURL = bootstrap.close_url;
    if (!closeURL) {
      window.close();
      return;
    }
    try {
      await fetch(closeURL, { method: 'POST' });
    } finally {
      window.close();
    }
  });
}

wireCloseButton();

const rootNode = document.getElementById('cr-preview-root');
if (rootNode) {
  ReactDOM.createRoot(rootNode).render(
    <React.StrictMode>
      <App bootstrap={bootstrap} />
    </React.StrictMode>,
  );
}
