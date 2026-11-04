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
