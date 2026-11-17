/**
 * identityGroups.ts supplies Chinese interface messages for identity membership, rate
 * overrides, balance discounts, and historical recharge gifts. These locale keys are consumed by the
 * corresponding pages and controls, with interpolation values provided by the
 * component displaying each message.
 */

export default {
  title: '身份分组',
  ordinary: '普通分组',
  members: '管理成员',
  rates: '设置倍率',
  count: '{count} 位成员',
  discount:
    '未开启“原价”的分组，余额消费按默认倍率的 {percent}% 计费。个人专属倍率和 AHHilAI 倍率优先。',
  pricing: '统一配置各普通分组的消费倍率，个人专属倍率优先。',
  search: '搜索用户名、邮箱或用户 ID',
  searchButton: '搜索',
  addMode: '查找并添加用户',
  memberMode: '查看已有成员',
  add: '加入身份组',
  remove: '移出身份组',
  selected: '已选择 {count} 位用户',
  replace: '95折和9折只能二选一，加入另一档会替换现有身份；AHHilAI 可以同时拥有。',
  previous: '上一页',
  next: '下一页',
  page: '第 {page} 页，共 {total} 位用户',
  empty: '没有匹配的用户',
  inherit: '继承',
  defaultRate: '默认倍率',
  identityRate: '身份倍率',
  group: '分组',
  clear: '全部继承',
  rateHelp: '空值继承普通分组倍率，0 表示免费。个人专属倍率优先。',
  save: '保存',
  close: '关闭',
  loading: '加载中…',
  saved: '已保存',
  failed: '操作失败，请重试。',
  invalid: '请输入非负的有效倍率。',
  assign: '分配身份',
  filter: '全部身份',
  benefits: '我的身份权益',
  none: '暂无特殊身份',
  base: '基础额度',
  gift: '赠送额度',
  credited: '到账额度',
  refund: '退款时按比例收回赠送额度。',
  select: '选择用户',
}
