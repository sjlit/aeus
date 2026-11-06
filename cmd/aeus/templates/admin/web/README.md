# __AEUS_NAME__ web

Vue 3 + Element Plus + `@sjlit/rest-ui` management console.

## Development

The Go backend serves the API on `:8080`; Vite proxies `/api` to it.

```bash
# terminal 1 (project root)
make run

# terminal 2
npm install
npm run dev
```

Open <http://localhost:5173>. Production builds use an empty
`VITE_API_BASE` and are served by the Go binary from `web/dist`.

To debug a local `@sjlit/rest-ui` checkout, set `REST_UI_LOCAL=1` and
`REST_UI_ROOT` to that checkout (absolute, or relative to the current
directory). The dev server then aliases `@sjlit/rest-ui` to its `src/`
and enables HMR. Setting only `REST_UI_LOCAL` fails fast instead of
falling back to a machine-specific path.

## Branding

The sidebar, login page and browser title share `public/app-config.js`:

```js
window.__AEUS_CONFIG__ = {
  appName: '__AEUS_NAME__',
  appMark: '',
}
```

`appName` is the product name; `appMark` is the character in the brand
square (empty means the first character of `appName`). Vite copies
`public/` to `dist/`, so you can edit `dist/app-config.js` after a build
and reload without rebuilding.

## Typography

Font sizes come from the `--fs-*` tokens in `src/styles/style.scss`
(11/12/13/14/16/20/22/32/36/64). New styles should use those tokens
instead of raw `px`; 11px is reserved for Latin/mono captions, kbd and
chart ticks, while Chinese body text is at least 12px. Small icons use
the nearest token; decorative 15/18/21/22 icons keep their raw sizes.
Chinese font weights come from the installed system font, so do not rely
on weight alone for hierarchy.

## Commands

```bash
npm run dev        # vite dev server
npm run typecheck  # vue-tsc
npm test           # vitest
npm run build      # typecheck + dist/
npm run preview    # preview the production build
```

Routes are hash-based. Business routes come from `GET /user/menus`;
`view_path` selects the file under `src/views`.
