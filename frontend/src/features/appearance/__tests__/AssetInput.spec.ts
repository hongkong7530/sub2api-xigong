import {afterEach,describe,it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
import AssetInput from '../AssetInput.vue'
const {upload}=vi.hoisted(()=>({upload:vi.fn()}))
vi.mock('@/api/siteContent',()=>({siteContent:{uploadAsset:upload}}))
vi.mock('vue-i18n',()=>({useI18n:()=>({t:(k:string)=>k})}))
afterEach(()=>vi.clearAllMocks())
async function choose(w:ReturnType<typeof mount>,file:File){const input=w.get('input[type=file]');Object.defineProperty(input.element,'files',{value:[file],configurable:true});await input.trigger('change')}
describe('asset upload ownership',()=>{
 it('ignores a pending upload after editing its target',async()=>{
  let finish!:(v:{url:string})=>void;upload.mockReturnValue(new Promise(r=>{finish=r}))
  const w=mount(AssetInput,{props:{modelValue:''}});await choose(w,new File(['png'],'a.png',{type:'image/png'}));await w.get('input[type=text]').setValue('/new.jpg');finish({url:'/old-upload.jpg'});await flushPromises();expect(w.emitted('update:modelValue')).toEqual([['/new.jpg']]);w.unmount()
 })
 it('does not restore a value after unmount',async()=>{
  let finish!:(v:{url:string})=>void;upload.mockReturnValue(new Promise(r=>{finish=r}))
  const w=mount(AssetInput,{props:{modelValue:''}});await choose(w,new File(['png'],'a.png',{type:'image/png'}));w.unmount();finish({url:'/stale.jpg'});await flushPromises();expect(w.emitted('update:modelValue')).toBeUndefined()
 })
 it('rejects active images before upload',async()=>{const w=mount(AssetInput);await choose(w,new File(['svg'],'a.svg',{type:'image/svg+xml'}));expect(upload).not.toHaveBeenCalled();expect(w.get('[role=alert]').text()).toContain('uploadLimit');w.unmount()})
})
