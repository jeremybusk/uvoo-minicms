  scancode -cl \
    --ignore ".git" \
    --ignore "web/node_modules" \
    --ignore "web/dist" \
    --ignore "dist" \
    --ignore "bin" \
    --ignore "scancode-report.json" \
    --json-pp scancode-source-report.json ./
