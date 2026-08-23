// Registers @testing-library/jest-dom's matchers (toHaveTextContent etc.)
// with vitest's expect, including their types.
import '@testing-library/jest-dom/vitest';

// jsdom does not implement scrollTo; the app calls it for scroll restoration.
window.scrollTo = () => {};
