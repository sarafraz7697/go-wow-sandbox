// Independent client-side SRP equations. Test credentials only.
// Run: node tools/srp-vectors.mjs > internal/auth/srp/testdata/vectors.json
import {createHash} from 'node:crypto';
const N=BigInt('0x894B645E89E1535BBDAD5B8B290650530801B18EBFBF5E8FAB3C82872A3E9BB7');
const H=(...b)=>createHash('sha1').update(Buffer.concat(b)).digest();
const le=(x,n)=>{const b=Buffer.alloc(n);for(let i=0;i<n;i++){b[i]=Number(x&255n);x>>=8n;}return b;};
const num=b=>BigInt('0x'+Buffer.from(b).reverse().toString('hex'));
const pow=(a,b)=>{let r=1n;a=((a%N)+N)%N;while(b){if(b&1n)r=r*a%N;a=a*a%N;b>>=1n;}return r;};
const name='TEST',password='PASSWORD';
const salt=Buffer.from(Array.from({length:32},(_,i)=>i===31?0:i+1));
const x=num(H(salt,H(Buffer.from(name+':'+password))));
const v=pow(7n,x);
function vector(a,b){
 const A=le(pow(7n,a),32),B=le((3n*v+pow(7n,b))%N,32);
 const u=num(H(A,B));
 const S=le(pow(num(B)-3n*pow(7n,x),a+u*x),32);
 const e=H(Buffer.from(S.filter((_,i)=>i%2===0))),o=H(Buffer.from(S.filter((_,i)=>i%2===1)));
 const K=Buffer.alloc(40);for(let i=0;i<20;i++){K[2*i]=e[i];K[2*i+1]=o[i];}
 const hn=H(le(N,32)),hg=H(Buffer.from([7]));
 const M=H(Buffer.from(hn.map((x,i)=>x^hg[i])),H(Buffer.from(name)),salt,A,B,K);
 const M2=H(A,M,K);
 return {name,password,salt:salt.toString('hex'),verifier:le(v,32).toString('hex'),secret:le(b,19).toString('hex'),A:A.toString('hex'),B:B.toString('hex'),key:K.toString('hex'),proof:M.toString('hex'),serverProof:M2.toString('hex'),S:S.toString('hex')};
}
const result=[{label:'small A and padded salt',...vector(1n,17n)}];
for(let b=1n;;b++){const v=vector(37n,b);if(v.B.endsWith('00')){result.push({label:'padded B',...v});break;}}
for(let a=2n;;a++){const v=vector(a,17n);if(v.proof.endsWith('00')){result.push({label:'padded proof',...v});break;}}
for(let a=2n;;a++){const v=vector(a,17n);if(v.S.endsWith('00')){result.push({label:'padded shared secret',...v});break;}}
console.log(JSON.stringify(result,null,2));
