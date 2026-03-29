import { deflateSync } from 'node:zlib';
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';

function createPng(color: [number, number, number]): Buffer {
  const width = 32;
  const height = 32;
  
  const signature = Buffer.from([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
  
  const ihdrData = Buffer.alloc(13);
  ihdrData.writeUInt32BE(width, 0);
  ihdrData.writeUInt32BE(height, 4);
  ihdrData.writeUInt8(8, 8);
  ihdrData.writeUInt8(6, 9);
  ihdrData.writeUInt8(0, 10);
  ihdrData.writeUInt8(0, 11);
  ihdrData.writeUInt8(0, 12);
  const ihdr = createChunk('IHDR', ihdrData);
  
  const rawData = Buffer.alloc(height * (width * 4 + 1));
  const cx = width / 2;
  const cy = height / 2;
  const radius = 14;
  
  for (let y = 0; y < height; y++) {
    rawData[y * (width * 4 + 1)] = 0;
    
    for (let x = 0; x < width; x++) {
      const idx = y * (width * 4 + 1) + 1 + x * 4;
      const dx = x - cx;
      const dy = y - cy;
      const dist = Math.sqrt(dx * dx + dy * dy);
      
      let r = 0, g = 0, b = 0, a = 0;
      
      if (dist <= radius) {
        const alpha = dist > radius - 1 ? 1 - (dist - (radius - 1)) : 1;
        
        r = color[0];
        g = color[1];
        b = color[2];
        a = Math.floor(255 * alpha);
        
        const isS = drawS(x, y, cx, cy);
        if (isS) {
            r = 255; g = 255; b = 255;
        }
      }
      
      rawData[idx] = r;
      rawData[idx + 1] = g;
      rawData[idx + 2] = b;
      rawData[idx + 3] = a;
    }
  }
  
  function drawS(x: number, y: number, cx: number, cy: number): boolean {
    if (x >= cx - 4 && x <= cx + 4 && y >= cy - 8 && y <= cy - 6) return true;
    if (x >= cx - 6 && x <= cx - 4 && y >= cy - 8 && y <= cy - 2) return true;
    if (x >= cx - 4 && x <= cx + 4 && y >= cy - 2 && y <= cy + 0) return true;
    if (x >= cx + 4 && x <= cx + 6 && y >= cy + 0 && y <= cy + 6) return true;
    if (x >= cx - 4 && x <= cx + 4 && y >= cy + 6 && y <= cy + 8) return true;
    if (x >= cx + 4 && x <= cx + 6 && y >= cy - 8 && y <= cy - 6) return true;
    if (x >= cx - 6 && x <= cx - 4 && y >= cy + 6 && y <= cy + 8) return true;

    return false;
  }
  
  const compressed = deflateSync(rawData);
  const idat = createChunk('IDAT', compressed);
  
  const iend = createChunk('IEND', Buffer.alloc(0));
  
  return Buffer.concat([signature, ihdr, idat, iend]);
}

function createChunk(type: string, data: Buffer): Buffer {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length, 0);
  
  const typeBuf = Buffer.from(type);
  const chunkData = Buffer.concat([typeBuf, data]);
  
  const crc = crc32(chunkData);
  const crcBuf = Buffer.alloc(4);
  crcBuf.writeUInt32BE(crc, 0);
  
  return Buffer.concat([length, chunkData, crcBuf]);
}

const crcTable = new Uint32Array(256);
for (let i = 0; i < 256; i++) {
  let c = i;
  for (let j = 0; j < 8; j++) {
    c = (c & 1) ? (0xEDB88320 ^ (c >>> 1)) : (c >>> 1);
  }
  crcTable[i] = c;
}

function crc32(buf: Buffer): number {
  let crc = 0xFFFFFFFF;
  for (let i = 0; i < buf.length; i++) {
    crc = crcTable[(crc ^ buf[i]) & 0xFF] ^ (crc >>> 8);
  }
  return (crc ^ 0xFFFFFFFF) >>> 0;
}

const COLOR_IDLE: [number, number, number] = [128, 128, 128];
const COLOR_CONNECTED: [number, number, number] = [34, 197, 94];
const COLOR_ERROR: [number, number, number] = [239, 68, 68];

const assetsDir = import.meta.dir;

writeFileSync(join(assetsDir, 'tray-idle.png'), createPng(COLOR_IDLE));
console.log('Created tray-idle.png');

writeFileSync(join(assetsDir, 'tray-connected.png'), createPng(COLOR_CONNECTED));
console.log('Created tray-connected.png');

writeFileSync(join(assetsDir, 'tray-error.png'), createPng(COLOR_ERROR));
console.log('Created tray-error.png');

console.log('All icons generated successfully!');