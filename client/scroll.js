/*
 * Various page scrolling logic
 */

let $ = require('jquery'),
    Backbone = require('backbone'),
    main = require('./main'),
    state = main.state;

const PAGE_BOTTOM = -1;

let nestLevel = 0,
	lockTarget, lockKeyHeight, $lockTarget, $lockIndicator, lockedManually;

// Checks if we're at the bottom of page at the moment    
let at_bottom = function() {
	return window.scrollY + window.innerHeight >= main.$doc.height() - 5;
};
if (window.scrollMaxY !== undefined) {
	at_bottom = function () {
		return window.scrollMaxY <= window.scrollY;
	};
}

// Sets the scroll lock position (to a post or to bottom of window)
function set_lock_target(num, manually) {
	lockedManually = manually;
	if (!num && at_bottom())
		num = PAGE_BOTTOM;
	if (num == lockTarget)
		return;
	lockTarget = num;
	const bottom = lockTarget == PAGE_BOTTOM;
	if ($lockTarget)
		$lockTarget.removeClass('scroll-lock');
	if (num && !bottom && manually)
		$lockTarget = $('#' + num).addClass('scroll-lock');
	else
		$lockTarget = null;

	let $ind = $lockIndicator;
	if ($ind) {
		const visible = bottom || manually;
		$ind.css({
			visibility: visible ? 'visible' : 'hidden'
		});
		if (bottom)
			$ind.text('Locked to bottom');
		else if (num) {
			$ind.empty().append($('<a/>', {
				text: '>>' + num,
				href: '#' + num
			}));
		}
	}
}

/* 
 * Logic for locking position to bottom of thread
 * Records the original scroll position before function is called
 * Adjusts the scroll position back to original after function executes.
 * Use for every action that would change length of a thread.
 */
function followLock(func) {
	var lockHeight,
		locked = lockTarget,
		$post;
	if (locked === PAGE_BOTTOM)
		lockHeight = main.$doc.height();
	else if (locked) {
		$post = $('#' + locked);
		const r = $post.length && $post[0].getBoundingClientRect();
		if (r && r.bottom > 0 && r.top < window.innerHeight)
			lockHeight = r.top;
		else
			locked = false;
	}

	let ret;
	try {
		nestLevel++;
		ret = func.call(this);
	}
	catch (e) {}

    //If we aren't in a thread, don't lock to bottom
    if (!state.page.get('thread'))
        return;
	if (locked === PAGE_BOTTOM) {
		const height = main.$doc.height();
		if (height > lockHeight - 10)
			window.scrollBy(0, height - lockHeight + 10);
	}
	else if (locked && lockTarget === locked) {
		const newY = $post[0].getBoundingClientRect().top;
		window.scrollBy(0, newY - lockHeight);
	}

	return ret;
}
main.comply('scroll:follow', followLock);

/* Uncomment when certain of menuHandler things being functional
 * Locks to post
menuHandlers.Focus = function (model) {
	var num = model && model.id;
	set_lock_target(num, true);
};
	//Unlocks from post or bottom
menuHandlers.Unfocus = function () {
	set_lock_target(null);
};
*/

//Check if user scrolled to the bottom every time they scroll
function scroll_shita() {
	if (state.page.get('thread') && (!lockTarget || lockTarget == PAGE_BOTTOM))
		set_lock_target(null);
}

function find_lock() {
	let $ind = main.$threads.children('#lock');
	$lockIndicator = $ind.length ? $ind : null;
}

find_lock();
scroll_shita();
main.$doc.scroll(scroll_shita);
// Reapply lock visibility on page change
state.page.on('change', function() {
	find_lock();
	scroll_shita();
});

// If a post is a locked target and is removed, unlock from post
state.posts.on('remove', function(model) {
	if (model.get('num') == lockTarget)
		set_lock_target(null);
});

// Account for banner height, when scrolling to an anchor
function aboveBanner (){
	if (!/^#\d+$/.test(location.hash))
		return;
	let $anchor = $(location.hash);
	if (!$anchor.length)
		return;
	$(window).scrollTop($anchor.offset().top - $('#banner').height());
}
main.comply('scroll:aboveBanner', aboveBanner);
window.onload = aboveBanner;
