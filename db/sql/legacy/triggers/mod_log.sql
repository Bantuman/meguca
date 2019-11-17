create or replace function after_mod_log_insert()
returns trigger
language plpgsql
as $$
declare
	op bigint;
begin
	if new.post_id != 0 then
		insert into post_moderation (post_id, type, "by", length, data)
			values (new.post_id, new.type, new."by", new.length, new.data);
		update posts
			set moderated = true
			where id = new.post_id
			returning posts.op into op;
		perform pg_notify(
			'post.moderated',
			concat_ws(',', thread_board(op), op, post_id, new.id)
		);

		-- Posts bump threads only on creation and closure
		perform bump_thread(op, bump_time => true);
	end if;
	return null;
end;
$$;
