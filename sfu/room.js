'use strict';

const mediaCodecs = [
  {
    kind: 'audio',
    mimeType: 'audio/opus',
    clockRate: 48000,
    channels: 2,
  },
  {
    kind: 'video',
    mimeType: 'video/VP8',
    clockRate: 90000,
    parameters: { 'x-google-start-bitrate': 1000 },
  },
  {
    kind: 'video',
    mimeType: 'video/VP9',
    clockRate: 90000,
    parameters: { 'profile-id': 2, 'x-google-start-bitrate': 1000 },
  },
  {
    kind: 'video',
    mimeType: 'video/h264',
    clockRate: 90000,
    parameters: {
      'packetization-mode': 1,
      'profile-level-id': '4d0032',
      'level-asymmetry-allowed': 1,
      'x-google-start-bitrate': 1000,
    },
  },
];

class Room {
  constructor(roomId, worker) {
    this.roomId = roomId;
    this.worker = worker;
    this.router = null;
    this.peers = new Map();
  }

  async init() {
    this.router = await this.worker.createRouter({ mediaCodecs });
    console.log(`[Room ${this.roomId}] router created`);
  }

  addPeer(peerId, ws) {
    this.peers.set(peerId, {
      ws,
      transports: new Map(),
      producers: new Map(),
      consumers: new Map(),
    });
    console.log(`[Room ${this.roomId}] peer joined: ${peerId}, total: ${this.peers.size}`);
  }

  // Исправлено: явно закрываем consumers и producers перед транспортами
  removePeer(peerId) {
    const peer = this.peers.get(peerId);
    if (!peer) return;

    for (const consumer of peer.consumers.values()) {
      try { consumer.close(); } catch (_) {}
    }
    for (const producer of peer.producers.values()) {
      try { producer.close(); } catch (_) {}
    }
    for (const transport of peer.transports.values()) {
      try { transport.close(); } catch (_) {}
    }

    this.peers.delete(peerId);
    this.broadcast(peerId, { type: 'peerLeft', peerId });
    console.log(`[Room ${this.roomId}] peer left: ${peerId}, total: ${this.peers.size}`);
  }

  getRouterRtpCapabilities() {
    return this.router.rtpCapabilities;
  }

  async createTransport(peerId) {
    const transport = await this.router.createWebRtcTransport({
      listenIps: [
        {
          ip: process.env.MEDIASOUP_LISTEN_IP || '0.0.0.0',
          announcedIp: process.env.MEDIASOUP_ANNOUNCED_IP || '127.0.0.1',
        },
      ],
      enableUdp: true,
      enableTcp: true,
      preferUdp: true,
      initialAvailableOutgoingBitrate: 1000000,
    });

    const peer = this.peers.get(peerId);
    if (peer) peer.transports.set(transport.id, transport);

    transport.on('dtlsstatechange', (state) => {
      if (state === 'closed') transport.close();
    });

    return {
      id: transport.id,
      iceParameters: transport.iceParameters,
      iceCandidates: transport.iceCandidates,
      dtlsParameters: transport.dtlsParameters,
    };
  }

  async connectTransport(peerId, transportId, dtlsParameters) {
    const peer = this.peers.get(peerId);
    if (!peer) throw new Error('peer not found');

    const transport = peer.transports.get(transportId);
    if (!transport) throw new Error('transport not found');

    await transport.connect({ dtlsParameters });
  }

  async produce(peerId, transportId, kind, rtpParameters) {
    const peer = this.peers.get(peerId);
    if (!peer) throw new Error('peer not found');

    const transport = peer.transports.get(transportId);
    if (!transport) throw new Error('transport not found');

    const producer = await transport.produce({ kind, rtpParameters });
    peer.producers.set(producer.id, producer);

    producer.on('transportclose', () => producer.close());

    this.broadcast(peerId, {
      type: 'newProducer',
      peerId,
      producerId: producer.id,
      kind,
    });

    return producer.id;
  }

  async consume(peerId, producerPeerId, producerId, rtpCapabilities) {
    const peer = this.peers.get(peerId);
    if (!peer) throw new Error('peer not found');

    if (!this.router.canConsume({ producerId, rtpCapabilities })) {
      throw new Error('cannot consume');
    }

    let recvTransport = null;
    for (const t of peer.transports.values()) {
      if (t.appData && t.appData.direction === 'recv') {
        recvTransport = t;
        break;
      }
    }
    if (!recvTransport) throw new Error('recv transport not found');

    const consumer = await recvTransport.consume({
      producerId,
      rtpCapabilities,
      paused: false,
    });

    peer.consumers.set(consumer.id, consumer);

    consumer.on('transportclose', () => consumer.close());
    consumer.on('producerclose', () => {
      consumer.close();
      peer.consumers.delete(consumer.id);
      this.sendTo(peerId, { type: 'consumerClosed', consumerId: consumer.id });
    });

    return {
      consumerId: consumer.id,
      producerId,
      kind: consumer.kind,
      rtpParameters: consumer.rtpParameters,
    };
  }

  getProducers(excludePeerId) {
    const producers = [];
    for (const [peerId, peer] of this.peers.entries()) {
      if (peerId === excludePeerId) continue;
      for (const [producerId, producer] of peer.producers.entries()) {
        producers.push({ peerId, producerId, kind: producer.kind });
      }
    }
    return producers;
  }

  sendTo(peerId, message) {
    const peer = this.peers.get(peerId);
    if (peer && peer.ws.readyState === 1) {
      peer.ws.send(JSON.stringify(message));
    }
  }

  broadcast(fromPeerId, message) {
    for (const [peerId] of this.peers.entries()) {
      if (peerId !== fromPeerId) this.sendTo(peerId, message);
    }
  }

  isEmpty() {
    return this.peers.size === 0;
  }
}

module.exports = Room;