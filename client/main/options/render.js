import {_, isMobile, options} from 'main'

const lang = main.lang.opts

/**
 * Render the options panel
 */
export default function render() {
    let html = `<div class="bmodal glass" id="options-panel">`
        + `<ul class="option_tab_sel">`
    const {tabs} = lang,
        opts = []

    // Render tab butts
    for (let i = 0; i < tabs.length; i++) {
        // Pick the options for this specific tab, according to current
        // template and server configuration
        opts[i] = _.filter(options, opt =>
            opt.tab === i
                && (opt.load === undefined  || opt.load)
                && !opt.hidden)

        if (!opts[i].length) {
            continue
        }
        html += `<li><a data-content="tab-${i}"`

        // Highlight the first tabButt by default
        if (i === 0) {
            html += ' class="tab_sel"'
        }

        html += `>${tabs[i]}</a></li>`;
    }

    html += '</ul><ul class="option_tab_cont">'
    for (let i = 0; i < opts.length; i++) {
        html += renderTab(opt, i)
    }
    html += '</ul></div>'

    return html
}

/**
 * Render tab contents
 */
function renderTab(opt, i) {
    if (!opts[i].length) {
        return ''
    }
    let html = ""
    html += `<li class="tab-${i}`

    // Show the first tab by default
    if (i === 0) {
        html += ' tab_sel'
    }
    html += '">'

    // Render the actual options
    for (let opt of opts[i]) {
        html += renderOption(opt)
    }

    if (i === 0) {
        html += renderExtras()
    }
    html += '</li>'

    return html
}

/**
 * Render a single option from it's schema
 */
function renderOption(opt) {
	let html = ''
	const isShortcut = opt.type === 'shortcut',
		isList = opt.type instanceof Array,
		isCheckbox = opt.type === 'checkbox' || opt.type === undefined,
		isNumber = opt.type === 'number',
		isImage = opt.type === 'image'
	if (isShortcut) {
        html += 'Alt+'
    }
	if (!isList) {
		html += '<input'
		if (isCheckbox || isImage)
			html += ` type="${(isCheckbox ? 'checkbox' : 'file')}"`
		else if (isNumber)
			html += ' style="width: 4em;" maxlength="4"'
		else if (isShortcut)
			html += ' maxlength="1"'
	} else {
        html += '<select'
    }

	const [label,title] = lang[opt.id]
	html += ` id="${opt.id}" title="${title}">`

	if (isList) {
		for (let item of opt.type) {
			html += `<option value="${item}">${lang[item] || item}</option>`
		}
		html += '</select>'
	}
	html += `<label for="${opt.id}" title="${title}">${label}</label><br>`

	return html
}

/**
 * Hidden post reset, Export and Import links to first tab
 */
function renderExtras() {
	let html = '<br>'
	const links = ['export', 'import', 'hidden']
    for (let id of links) {
        const [label, title] = lang[id]
        html += `<a id="${id}" title="${ln[1]}">${ln[0]}</a> `
    }

    // Hidden file input for uploading the JSON
    const attrs = {
        type: 'file',
        id: 'importSettings',
        name: "Import Settings"
    }
	html += common.parseHTML`<input ${attrs}>`

    return html
}
