export { Post } from "./model"
export { default as PostView } from "./view"
export {
	postEvent, postSM, postState, postModel, postForm, FormModel, identity
} from "./posting"
export { default as ImageHandler, toggleExpandAll, setExpandAll } from "./images"
export { clearHidden } from "./hide"
export { renderTime, thumbPath } from "./render"
export { default as PostCollection } from "./collection"

import initEtc from "./etc"
import initPosting from "./posting"
import initMenu from "./menu"
import initInlineExpansion from "./inlineExpansion"
import initHover from "./hover"

export default () => {
	initEtc()
	initPosting()
	initMenu()
	initInlineExpansion()
	initHover()
}
