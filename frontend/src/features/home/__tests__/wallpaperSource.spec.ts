import {describe,it,expect} from 'vitest'
import {wallpaperSource,wallpaperFocus} from '../wallpaperSource'
describe('device wallpaper sources',()=>{
 it('uses independent images and explicit zero focus',()=>{
  const w={id:'scene-4',focus_x:70,focus_y:45,desktop_url:'/api/v1/site-content/assets/wide',mobile_url:'https://example.com/tall.jpg',mobile_focus_x:0,mobile_focus_y:0}
  expect(wallpaperSource(w)).toContain('/assets/wide');expect(wallpaperSource(w,true)).toBe('https://example.com/tall.jpg');expect(wallpaperFocus(w,true)).toEqual({x:0,y:0})
 })
 it('falls back from cleared mobile to desktop then built-in',()=>{
  expect(wallpaperSource({id:'scene-2',focus_x:20,focus_y:30,desktop_url:'/wide.jpg',mobile_url:''},true)).toBe('/wide.jpg')
  expect(wallpaperSource({id:'scene-2',focus_x:20,focus_y:30},true)).toBe('/experience/wallpapers/scene-2-mobile.webp')
 })
 it('rejects active URLs',()=>{expect(wallpaperSource({id:'scene-1',focus_x:50,focus_y:50,desktop_url:'javascript:alert(1)'})).toBe('/experience/wallpapers/scene-1-desktop.webp')})
})
