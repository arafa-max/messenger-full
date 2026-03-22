'use strict';

const mediasoup = require('mediasoup');
const { WebSocketServer } = require('ws');
const { createServer } = require('http');
const { v4: uuidv4 } = require('uuid');
const Room = require('./room');

const PORT = process.env.SFU_PORT || 4000;
const NUM_WORKERS = Math.min(require('os').cpus().length, 4);

const workers = [];
let workerIdx = 0;
const rooms = new Map();

async function createWorkers() {
  for (let i = 0; i < NUM_WORKERS; i++) {
    const worker = await mediasoup.createWorker({
      logLevel: 'warn',
      rtcMinPort: 40000,
      rtcMaxPort: 49999,
    });

    worker.on('died', () => {
      console.error(`mediasoup worker died, pid: ${worker.pid}`);
      process.exit(1);
    });

    workers.push(worker);
    console.log(`worker created, pid: ${worker.pid}`);
  }
}

function getWorker() {
  const worker = workers[workerIdx];
  workerIdx = (workerIdx + 1) % workers.length;
  return worker;
}

async function getOrCreateRoom(roomId) {
  if (rooms.has(roomId)) return rooms.get(roomId);
  const room = new Room(roomId, getWorker());
  await room.init();
  rooms.set(roomId, room);
  return room;
}

async function handleMessage(ws, peerId, room, message) {
  const { type, data } = message;

  switch (type) {
    case 'getRouterRtpCapabilities': {
      ws.send(JSON.stringify({
        type: 'routerRtpCapabilities',
        data: room.getRouterRtpCapabilities(),
      }));
      break;
    }
    case 'createTransport': {
      const transport = await room.createTransport(peerId);
      const peer = room.peers.get(peerId);
      if (peer) {
        const t = peer.transports.get(transport.id);
        if (t) t.appData = { direction: data.direction };
      }
      ws.send(JSON.stringify({ type: 'transportCreated', data: transport }));
      break;
    }
    case 'connectTransport': {
      await room.connectTransport(peerId, data.transportId, data.dtlsParameters);
      ws.send(JSON.stringify({ type: 'transportConnected' }));
      break;
    }
    case 'produce': {
      const producerId = await room.produce(
        peerId, data.transportId, data.kind, data.rtpParameters
      );
      ws.send(JSON.stringify({ type: 'produced', data: { producerId } }));
      break;
    }
    case 'consume': {
      const consumerData = await room.consume(
        peerId, data.producerPeerId, data.producerId, data.rtpCapabilities
      );
      ws.send(JSON.stringify({ type: 'consumed', data: consumerData }));
      break;
    }
    case 'getProducers': {
      const producers = room.getProducers(peerId);
      ws.send(JSON.stringify({ type: 'producers', data: producers }));
      break;
    }
    default:
      console.warn(`unknown message type: ${type}`);
  }
}

async function main() {
  await createWorkers();

  // HTTP сервер: healthcheck + WebSocket на одном порту
  const httpServer = createServer((req, res) => {
    if (req.url === '/health') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        status: 'ok',
        rooms: rooms.size,
        workers: workers.length,
      }));
      return;
    }
    res.writeHead(404);
    res.end();
  });

  const wss = new WebSocketServer({ server: httpServer });
  console.log(`SFU listening on port ${PORT}`);

  wss.on('connection', async (ws, req) => {
    const url = new URL(req.url, `http://localhost`);
    const roomId = url.searchParams.get('roomId');
    const peerId = url.searchParams.get('peerId') || uuidv4();

    if (!roomId) {
      ws.close(4000, 'roomId required');
      return;
    }

    console.log(`[${roomId}] peer connected: ${peerId}`);

    let room;
    try {
      room = await getOrCreateRoom(roomId);
    } catch (err) {
      console.error('failed to get room:', err);
      ws.close(4001, 'room error');
      return;
    }

    room.addPeer(peerId, ws);

    const existingProducers = room.getProducers(peerId);
    if (existingProducers.length > 0) {
      ws.send(JSON.stringify({ type: 'producers', data: existingProducers }));
    }

    ws.on('message', async (raw) => {
      let message;
      try {
        message = JSON.parse(raw);
      } catch {
        console.warn('invalid JSON from peer:', peerId);
        return;
      }
      try {
        await handleMessage(ws, peerId, room, message);
      } catch (err) {
        console.error(`error handling [${message.type}]:`, err.message);
        ws.send(JSON.stringify({ type: 'error', message: err.message }));
      }
    });

    ws.on('close', () => {
      room.removePeer(peerId);
      if (room.isEmpty()) {
        rooms.delete(roomId);
        console.log(`[${roomId}] room deleted (empty)`);
      }
    });

    ws.on('error', (err) => {
      console.error(`ws error peer ${peerId}:`, err.message);
    });
  });

  httpServer.listen(PORT);

  // Graceful shutdown
  async function shutdown() {
    console.log('shutting down SFU...');
    for (const worker of workers) worker.close();
    httpServer.close(() => process.exit(0));
    setTimeout(() => process.exit(1), 5000);
  }
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
}

main().catch((err) => {
  console.error('fatal error:', err);
  process.exit(1);
});