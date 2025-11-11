INSERT INTO houses (ext_group_id, name) VALUES ('default-house', 'default')
ON CONFLICT DO NOTHING;

WITH h AS (SELECT id FROM houses WHERE ext_group_id='default-house')
INSERT INTO categories (house_id,name,weight)
SELECT h.id, v.name, v.weight
FROM h, (VALUES
 ('皿洗い',1.0),('ゴミ出し',1.2),('トイレ',1.6),('風呂',1.8),('掃除機',1.1),('買い出し',1.0)
) AS v(name,weight)
ON CONFLICT DO NOTHING;
