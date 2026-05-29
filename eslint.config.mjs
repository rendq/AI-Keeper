// Flat config (ESLint v9). Targets all TS/JS sources; tightened per-package
// when sub-packages land.
import tseslint from "@typescript-eslint/eslint-plugin";
import tsparser from "@typescript-eslint/parser";

export default [
    {
        ignores: [
            "**/node_modules/**",
            "**/dist/**",
            "**/build/**",
            "**/coverage/**",
            "proto/**",
            ".pdf-gen.js",
        ],
    },
    {
        files: ["**/*.{ts,tsx,js,mjs,cjs}"],
        languageOptions: {
            ecmaVersion: 2022,
            sourceType: "module",
            parser: tsparser,
        },
        plugins: {
            "@typescript-eslint": tseslint,
        },
        rules: {
            "no-unused-vars": "off",
            "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_" }],
            "no-console": ["warn", { allow: ["warn", "error"] }],
        },
    },
];
