# Build context: prototype/ (see offchain/custody/docker-compose.yml).
FROM node:22-alpine
WORKDIR /app/services/custody
COPY services/custody/package.json ./
RUN npm install --omit=dev --no-audit --no-fund && npm cache clean --force
# The service reuses the portal's byte-compatible envelope and keyed-identifier code.
COPY server/utils/envelope.ts server/utils/handles.ts /app/server/utils/
COPY services/custody/main.ts services/custody/ledger.ts ./
COPY services/custody/sql ./sql
USER node
EXPOSE 8100
CMD ["node_modules/.bin/tsx", "main.ts"]
