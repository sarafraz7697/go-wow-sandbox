// Independent JavaScript reference calculation. No Go code is called.
// Follows pinned AuthCrypt.cpp/HMACSHA1.cpp/BigNumber.cpp. Static vectors are
// checked into testdata so Go tests do not require Node or network access.
import { createHmac } from 'node:crypto';
import { writeFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
const seed = Buffer.from('38a78315f8922530719867b18c04e2aa','hex');
const clientHeaders = ['0004dc010000','000cdc010000','000437000000','2800ff030000','000aee010000','0004dc010000'];
const serverHeaders = ['0006ec01','0003ee01','00023b00','80000002','fffff103','0006dd01'];
const vectors = [];
for (const [name,session] of [
  ['full',Buffer.from(Array.from({length:40},(_,i)=>i+1))],
  ['high-zero',Buffer.from(Array.from({length:40},(_,i)=>i<37?i+1:0))],
  ['low-zero',Buffer.from(Array.from({length:40},(_,i)=>i<3?0:i+1))],
]) {
  let value=0n;
  for (let i=39;i>=0;i--) value=(value<<8n)|BigInt(session[i]);
  const minimal=[];
  while(value!==0n) {minimal.push(Number(value&255n));value>>=8n;}
  const key=createHmac('sha1',seed).update(Buffer.from(minimal)).digest();
  function encrypt(headers) {
    let index=0,previous=0;
    return headers.map(hex=>{
      const data=Buffer.from(hex,'hex');
      return Buffer.from([...data].map(b=>{
        const v=((b^key[index%20])+previous)&255; index++; previous=v; return v;
      })).toString('hex');
    });
  }
  vectors.push({name,session:session.toString('hex'),derived:key.toString('hex'),
    clientPlain:clientHeaders,clientEncrypted:encrypt(clientHeaders),
    serverPlain:serverHeaders,serverEncrypted:encrypt(serverHeaders)});
}
const target=new URL('../internal/protocol/world/tbc/testdata/',import.meta.url);
mkdirSync(target,{recursive:true});
writeFileSync(fileURLToPath(new URL('crypt.json',target)),JSON.stringify(vectors,null,2)+'\n');
console.log(`${vectors.length} independent TBC crypt vectors generated`);
