<script setup lang="ts">
import { computed, shallowRef, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useDocumentVisibility, useIntersectionObserver, useMediaQuery } from '@vueuse/core'
const props=defineProps<{text:string;active:boolean}>()
const host=shallowRef<HTMLElement>(),canvas=shallowRef<HTMLCanvasElement>(),assembling=shallowRef(false),visible=shallowRef(true)
const reduced=useMediaQuery('(prefers-reduced-motion: reduce)'),visibility=useDocumentVisibility()
const enabled=computed(()=>props.active&&!reduced.value&&visibility.value==='visible'&&visible.value)
useIntersectionObserver(host,([e])=>{visible.value=e.isIntersecting})
type Dot={x:number;y:number;sx:number;sy:number;delay:number;size:number;hue:number}
let dots:Dot[]=[],frame=0,start=0,alive=true,observer:ResizeObserver|undefined,epoch=0
function stop(){cancelAnimationFrame(frame);frame=0;assembling.value=false;canvas.value?.getContext('2d')?.clearRect(0,0,canvas.value.width,canvas.value.height)}
async function prepare(){
 const token=++epoch;stop();await nextTick();await document.fonts?.ready
 if(!alive||token!==epoch||!host.value||!canvas.value)return
 if(!enabled.value)return
 const width=host.value.clientWidth,height=host.value.clientHeight,dpr=Math.min(devicePixelRatio||1,1.5)
 if(!width||!height)return
 canvas.value.width=Math.round(width*dpr);canvas.value.height=Math.round(height*dpr)
 const sample=document.createElement('canvas');sample.width=Math.round(width);sample.height=Math.round(height)
 const ctx=sample.getContext('2d',{willReadFrequently:true});if(!ctx)return
 const title=host.value.querySelector('h1')!,style=getComputedStyle(title)
 ctx.font=`${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;ctx.textBaseline='middle';ctx.fillStyle='#fff'
 ctx.fillText(props.text,4,height/2,width-8)
 const data=ctx.getImageData(0,0,sample.width,sample.height).data,step=width<420?4:5;dots=[]
 for(let y=0;y<height;y+=step)for(let x=0;x<width;x+=step){if(data[(y*sample.width+x)*4+3]>100){const angle=Math.random()*Math.PI*2,r=width*(.25+Math.random()*.8);dots.push({x,y,sx:width/2+Math.cos(angle)*r,sy:height/2+Math.sin(angle)*r*.5,delay:Math.random()*.24,size:1.4+Math.random()*1.6,hue:155+Math.random()*45})}}
 if(enabled.value)replay()
}
function replay(){if(!enabled.value||!canvas.value||!dots.length)return;cancelAnimationFrame(frame);assembling.value=true;start=performance.now();frame=requestAnimationFrame(draw)}
function draw(now:number){
 if(!enabled.value||!canvas.value){stop();return}
 const ctx=canvas.value.getContext('2d');if(!ctx)return
 const dpr=Math.min(devicePixelRatio||1,1.5),elapsed=(now-start)/1600,width=canvas.value.width/dpr,height=canvas.value.height/dpr
 ctx.setTransform(dpr,0,0,dpr,0,0);ctx.clearRect(0,0,width,height)
 for(const dot of dots){const t=Math.min(1,Math.max(0,(elapsed-dot.delay)/.76)),ease=1-(1-t)**4,x=dot.sx+(dot.x-dot.sx)*ease,y=dot.sy+(dot.y-dot.sy)*ease;ctx.globalAlpha=Math.min(1,t*5)*(elapsed>1?.9:1);ctx.fillStyle=`hsl(${dot.hue} 85% ${76+t*22}%)`;ctx.fillRect(x,y,dot.size*(2.8-ease*1.8),dot.size*(2.8-ease*1.8))}
 if(elapsed<1.12)frame=requestAnimationFrame(draw);else stop()
}
watch(enabled,value=>{if(!value){epoch++;stop()}else void prepare()})
watch(()=>props.text,prepare)
onMounted(()=>{void prepare();observer=new ResizeObserver(()=>void prepare());if(host.value)observer.observe(host.value)})
onBeforeUnmount(()=>{alive=false;epoch++;stop();observer?.disconnect()})
</script>
<template>
 <div ref="host" class="particle-brand" :class="{'particle-brand--assembling':assembling}" @pointerenter="replay"><h1>{{ text }}</h1><canvas ref="canvas" aria-hidden="true" /></div>
</template>
<style scoped>
.particle-brand{position:relative;margin:25px -8px 22px;min-height:1.3em;font-size:clamp(50px,6.3vw,88px);line-height:1.35}.particle-brand h1{font-family:var(--xg-font-display);font-size:inherit;line-height:inherit;letter-spacing:-.06em;color:#efffff;text-shadow:0 2px 0 #7cbebc,0 4px 0 #315c72,0 1px 14px #11364e;transition:opacity .2s}.particle-brand--assembling h1{opacity:0}.particle-brand canvas{position:absolute;inset:0;width:100%;height:100%;pointer-events:none;overflow:visible}@media(max-width:700px){.particle-brand{font-size:clamp(46px,12.6vw,64px);margin:22px 0}}@media(prefers-reduced-motion:reduce){.particle-brand h1{opacity:1;transition:none}.particle-brand canvas{display:none}}
</style>
