import re
from sys import argv
from collections import defaultdict

# cur={42.52 428.35}
reCurs0 = re.compile(r'cur=\{([\d\.]+)\s([\d\.]+)\}')
lineCurs0 = 'text.go:716 showTrm: text.go:411  cur={42.52 428.35} CTM=[1.00 0.00 0.00 0.00 1.00 0.00 0.00 0.00 1.00]'
assert reCurs0.search(lineCurs0)

reCurs1 = re.compile(r'cur=([\d\.]+)\s([\d\.]+)')
lineCurs1 = 'fxState::shift: dx,dy=1.90 0.00 cur=278.04 426.45 -> 279.94 428.35'
assert reCurs1.search(lineCurs1)


def parseLine(line):
	m = reCurs0.search(line)
	if not m:
		m = reCurs1.search(line)
	if not m:
		return 0, 0, False
	assert m, '%d: >>>%s<<<' % (i, line)
	try:
		x = float(m.group(1))
		y = float(m.group(2))
	except Exception as e:
		print(e, line, m.groups())
		raise
	return x, y, True


def scan(path):
	print("scan: %s ----------------" % path)
	results = []
	with open(path, 'rt', errors='ignore') as f:
		for i, line in enumerate(f):
			line = line[:-1]
			if not line:
				continue
			x, y, found = parseLine(line)
			if not found:
				continue
			results.append((i, x, y, line))
	return results


R = 0.06
def rnd(x):
	return round(x / R) * R

def makeKey(x, y):
	return  '[%6.2f %6.2f]' % (rnd(x), rnd(y))

def asDict(results):
	d = defaultdict(list)
	for ixyline in results:
		_, x, y, _ = ixyline
		key = makeKey(x, y)
		d[key].append(ixyline)
	print('-------')
	for k in sorted(set(d))[:10]:
		if len(d[k]) > 1:
			print(k, len(d[k]))
	return d


results1 = scan(argv[1])
results2 = scan(argv[2])
keys1 = set(asDict(results1))
keys2 = set(asDict(results2))
print('%s %d matches %d unique' % (argv[1], len(results1), len(keys1)))
print('%s %d matches %d unique' % (argv[2], len(results2), len(keys2)))
print('============================')


matching = []
unmatching = []
for ixyline in results1:
	i, x, y, line = ixyline
	key = makeKey(x, y)
	if key in keys2:
		matching.append(ixyline)
	else:
		unmatching.append(ixyline)


print('%4d of %4d = %4.1f%% match  ' % (len(matching), len(results1), len(matching) / len(results1) * 100.0))
print('%4d of %4d = %4.1f%% unmatch' % (len(unmatching), len(results1), len(unmatching) / len(results1) * 100.0))

print('============================')

for ixyline in unmatching[:20]:
	i, x, y, line = ixyline
	# key = makeKey(x, y)
	# print('%4d: %s key=%s' % (i, line, key))
	print('%4d: %s' % (i, line))
	